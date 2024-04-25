// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: BUSL-1.1

package testutil

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shoenig/test/must"
)

func TestWait_WaitForFilesUntil(t *testing.T) {

	N := 10

	tmpDir := t.TempDir()

	var files []string
	for i := 1; i < N; i++ {
		files = append(files, filepath.Join(
			tmpDir, fmt.Sprintf("test%d.txt", i),
		))
	}

	go func() {
		for _, file := range files {
			t.Logf("Creating file %s ...", file)
			fh, createErr := os.Create(file)
			must.NoError(t, createErr)

			must.Close(t, fh)
			must.FileExists(t, file)

			time.Sleep(250 * time.Millisecond)
		}
	}()

	duration := 5 * time.Second
	t.Log("Waiting 5 seconds for files ...")
	WaitForFilesUntil(t, files, duration)
}
