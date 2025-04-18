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
 * @project fatima-core
 * @author jin
 * @date 23. 4. 14. 오후 6:11
 */

package main

import (
	"flag"
	"fmt"
	"os"
)

var usage = `usage: %s [option] process_name
usage: %s version

golang fatima package builder

positional arguments:
  process_name          process(program) name

optional arguments:
  -c    CGO Enable
  -s    Strip library while CGO enable
`

var cgoEnable = false
var stripEnable = false
var version = "2.3.1"

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

	flag.BoolVar(&cgoEnable, "c", false, "CGO enable")
	flag.BoolVar(&stripEnable, "s", false, "CGO enable")

	flag.Parse()
	if len(flag.Args()) < 1 {
		flag.Usage()
		return
	}

	processName := flag.Args()[0]

	ctx, err := NewBuildContext(processName)
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
