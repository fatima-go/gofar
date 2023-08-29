/*
 * Copyright 2023 github.com/fatima-go
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * @project fatima-go
 * @author dave_01
 * @date 23. 8. 29. 오후 5:38
 */

package main

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

func TestReadGitInfo(t *testing.T) {
	path, _ := os.Getwd()
	gitInfo := readGitInfo(path)
	assert.True(t, gitInfo.Valid, "git info is not valid")
	fmt.Printf("BranchName : %s\n", gitInfo.BranchName)
	fmt.Printf("CommitHash : %s\n", gitInfo.CommitHash)
	fmt.Printf("LastCommitMessage : %s\n", gitInfo.LastCommitMessage)
}
