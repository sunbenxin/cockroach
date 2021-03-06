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
// Author: Raphael 'kena' Poss (knz@cockroachlabs.com)

package acceptance

import (
	"path/filepath"
	"testing"

	"golang.org/x/net/context"

	"github.com/cockroachdb/cockroach/pkg/acceptance/cluster"
	"github.com/cockroachdb/cockroach/pkg/util/log"
	"github.com/docker/docker/api/types/container"
)

const testGlob = "../cli/interactive_tests/test*.tcl"
const containerPath = "/go/src/github.com/cockroachdb/cockroach/cli/interactive_tests"

var cmdBase = []string{
	"/usr/bin/env",
	"COCKROACH_SKIP_UPDATE_CHECK=1",
	"/usr/bin/expect",
}

func TestDockerCLI(t *testing.T) {
	containerConfig := container.Config{
		Image: postgresTestImage,
		Cmd:   []string{"stat", cluster.CockroachBinaryInContainer},
	}
	ctx := context.Background()
	if err := testDockerOneShot(ctx, t, "cli_test", containerConfig); err != nil {
		t.Skipf(`TODO(dt): No binary in one-shot container, see #6086: %s`, err)
	}

	paths, err := filepath.Glob(testGlob)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatalf("no testfiles found (%v)", testGlob)
	}

	verbose := testing.Verbose() || log.V(1)
	for _, p := range paths {
		testFile := filepath.Base(p)
		testPath := filepath.Join(containerPath, testFile)
		t.Run(testFile, func(t *testing.T) {
			cmd := cmdBase
			if verbose {
				cmd = append(cmd, "-d")
			}
			cmd = append(cmd, "-f", testPath, cluster.CockroachBinaryInContainer)
			containerConfig.Cmd = cmd
			if err := testDockerOneShot(ctx, t, "cli_test", containerConfig); err != nil {
				t.Error(err)
			}
		})
	}
}
