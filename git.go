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
 * @date 23. 8. 29. 오후 5:35
 */

package main

import (
	"fmt"
	"github.com/go-git/go-git/v5"
	"strings"
)

const (
	refHeadPrefix = "refs/heads/"
)

var refHeadPrefixLen = len(refHeadPrefix)

type GitInfo struct {
	Valid             bool
	RepoUrl           string
	BranchName        string
	CommitHash        string
	LastCommitMessage string
}

func (g GitInfo) ToMap() map[string]string {
	m := make(map[string]string)
	if len(g.RepoUrl) > 0 {
		m["repo"] = g.RepoUrl
	}
	m["branch"] = g.BranchName
	m["commit"] = g.CommitHash
	m["message"] = g.LastCommitMessage
	return m
}

func readGitInfo(baseDir string) GitInfo {
	gitInfo := GitInfo{Valid: false}
	gitRepo, err := git.PlainOpen(baseDir)
	if err != nil {
		fmt.Printf("fail to open git %s : %s\n", baseDir, err.Error())
		return gitInfo
	}

	// retrieve head
	ref, err := gitRepo.Head()
	if err != nil {
		fmt.Printf("repo.Head error : %s", err.Error())
		return gitInfo
	}

	remote, err := gitRepo.Remote("origin")
	if err == nil && len(remote.Config().URLs[0]) > 0 {
		gitInfo.RepoUrl = remote.Config().URLs[0]
	}

	gitInfo.BranchName = ref.Name().String()
	// BranchName likes : refs/heads/enhancement/git_commit_message
	if len(gitInfo.BranchName) > refHeadPrefixLen && strings.HasPrefix(gitInfo.BranchName, refHeadPrefix) {
		gitInfo.BranchName = gitInfo.BranchName[refHeadPrefixLen:]
	}
	gitInfo.CommitHash = ref.Hash().String()
	// ... retrieves the commit history
	cIter, err := gitRepo.Log(&git.LogOptions{From: ref.Hash()})
	if err != nil {
		fmt.Printf("reference log loading error : %s", err.Error())
		return gitInfo
	}

	commit, err := cIter.Next()
	if err != nil {
		fmt.Printf("commit log iterating : %s", err.Error())
	} else {
		gitInfo.LastCommitMessage = commit.Message
	}

	gitInfo.Valid = true
	return gitInfo
}
