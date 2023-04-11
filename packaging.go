/*
 * //
 * // Licensed to the Apache Software Foundation (ASF) under one
 * // or more contributor license agreements.  See the NOTICE file
 * // distributed with p work for additional information
 * // regarding copyright ownership.  The ASF licenses p file
 * // to you under the Apache License, Version 2.0 (the
 * // "License"); you may not use p file except in compliance
 * // with the License.  You may obtain a copy of the License at
 * //
 * //   http://www.apache.org/licenses/LICENSE-2.0
 * //
 * // Unless required by applicable law or agreed to in writing,
 * // software distributed under the License is distributed on an
 * // "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * // KIND, either express or implied.  See the License for the
 * // specific language governing permissions and limitations
 * // under the License.
 * //
 * // @project fatima
 * // @author DeockJin Chung (jin.freestyle@gmail.com)
 * // @date 21. 8. 8. 오후 9:24
 * //
 */

package main

import (
	"flag"
	"fmt"
	"os"
)

var usage = `usage: %s process_name os_arc cgo
usage: %s version

golang fatima package builder

positional arguments:
  process_name          process(program) name
  os_arc                optional. e.g) linux_amd64
  cgo                   CC link e.g) x86_64-pc-linux-gcc
`

var version = "1.0.0"

func Gofar() {
	if len(os.Args) > 1 {
		if os.Args[1] == "version" {
			fmt.Printf("gofar version %s\n", version)
			return
		}
	}

	flag.Usage = func() {
		fmt.Printf(usage, os.Args[0], os.Args[0])
	}

	flag.Parse()
	if len(flag.Args()) < 1 {
		flag.Usage()
		return
	}

	osArc := ""
	processName := flag.Args()[0]
	if len(flag.Args()) >= 2 {
		osArc = flag.Args()[1]
	}
	cgoLink := ""
	if len(flag.Args()) >= 3 {
		cgoLink = flag.Args()[2]
	}

	ctx, err := NewBuildContext(processName, osArc, cgoLink)
	if err != nil {
		fmt.Fprintf(os.Stderr, "packaging error : %s", err.Error())
		return
	}

	ctx.Print()

	err = ctx.Packaging()
	if err != nil {
		fmt.Fprintf(os.Stderr, "gofar packaging fail : %s", err.Error())
	}
}
