// Copyright 2016 The Cockroach Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied. See the License for the specific language governing
// permissions and limitations under the License.
//
// Author: Irfan Sharif (irfansharif@cockroachlabs.com)

package distsqlrun

import (
	"golang.org/x/net/context"

	"github.com/cockroachdb/cockroach/pkg/sql/parser"
	"github.com/cockroachdb/cockroach/pkg/sql/sqlbase"
	"github.com/cockroachdb/cockroach/pkg/util/log"
)

type joinerBase struct {
	inputs []RowSource
	output RowReceiver
	ctx    context.Context

	joinType    joinType
	outputCols  columns
	onCond      exprHelper
	rowAlloc    sqlbase.EncDatumRowAlloc
	emptyLeft   sqlbase.EncDatumRow
	emptyRight  sqlbase.EncDatumRow
	combinedRow sqlbase.EncDatumRow
}

func (jb *joinerBase) init(
	flowCtx *FlowCtx,
	inputs []RowSource,
	output RowReceiver,
	outputCols []uint32,
	jType JoinType,
	leftTypes []sqlbase.ColumnType,
	rightTypes []sqlbase.ColumnType,
	expr Expression,
) error {
	jb.inputs = inputs
	jb.output = output
	jb.ctx = log.WithLogTag(flowCtx.Context, "Joiner", nil)
	jb.outputCols = columns(outputCols)
	jb.joinType = joinType(jType)
	jb.emptyLeft = make(sqlbase.EncDatumRow, len(leftTypes))
	for i := range jb.emptyLeft {
		jb.emptyLeft[i].Datum = parser.DNull
	}

	jb.emptyRight = make(sqlbase.EncDatumRow, len(rightTypes))
	for i := range jb.emptyRight {
		jb.emptyRight[i].Datum = parser.DNull
	}

	types := make([]sqlbase.ColumnType, 0, len(leftTypes)+len(rightTypes))
	types = append(types, leftTypes...)
	types = append(types, rightTypes...)

	return jb.onCond.init(expr, types, flowCtx.evalCtx)
}

// render evaluates the provided on-condition and constructs a row with columns
// from both rows as specified by the provided output columns. We expect left or
// right to be nil if there was no explicit "join" match, the on condition is
// then evaluated on a combinedRow with null values for the columns of the nil
// row. render returns a nil row if no row is to be emitted (eg. if join type is
// inner join and one of the given rows is nil). The returned boolean indicates
// whether or not the returned row failed the on condition.
func (jb *joinerBase) render(
	lrow, rrow sqlbase.EncDatumRow,
) (ret sqlbase.EncDatumRow, failedOnCond bool, err error) {
	lnil := lrow == nil
	rnil := rrow == nil
	if lnil {
		if jb.joinType == innerJoin || jb.joinType == leftOuter {
			return nil, false, nil
		}
		lrow = jb.emptyLeft
	}
	if rnil {
		if jb.joinType == innerJoin || jb.joinType == rightOuter {
			return nil, false, nil
		}
		rrow = jb.emptyRight
	}
	jb.combinedRow = append(jb.combinedRow[:0], lrow...)
	jb.combinedRow = append(jb.combinedRow, rrow...)
	res, err := jb.onCond.evalFilter(jb.combinedRow)
	if err != nil {
		return nil, false, err
	}
	if !res && !rnil && !lnil {
		// The on condition failed and we're trying to render a row that's been
		// successfully equality joined already. In this case, we need to
		// output a failed match row that contains just the left side, if we're
		// required to by the join style.
		if jb.joinType == innerJoin || jb.joinType == rightOuter {
			// If we're doing an inner or right outer join, we shouldn't return
			// a row: we don't want to output rows with NULL right sides. We
			// still need to notify the caller that the on condition failed,
			// though, because they'll want to render the corresponding right
			// side row by itself later.
			return nil, true, nil
		}
		jb.combinedRow = append(jb.combinedRow[:len(lrow)], jb.emptyRight...)
	}

	row := jb.rowAlloc.AllocRow(len(jb.outputCols))
	for i, col := range jb.outputCols {
		row[i] = jb.combinedRow[col]
	}
	return row, !res, nil
}
