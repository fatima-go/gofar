# gofar

gofar 는 golang 프로그램을 fatima 환경에 배포하기 위해 빌드를 도와주는 툴이다.

# Release

- v2.8.0 : 프로젝트 설정에 CGO 활성화(build.cgo) 지원 - -c 옵션 없이도 CGO 빌드, 프로젝트 설정의 알 수 없는 키는 에러 대신 경고 후 무시
- v2.7.0 : process 별 프로젝트 설정(.gofar/<process_name>.yaml) 지원 - 한 프로젝트의 여러 process 를 각자의 설정으로 빌드
- v2.6.0 : 프로젝트별 설정(.gofar/gofar.yaml) 지원 - 바이너리별 빌드 제외, strip, profile 별 설치 정책
- v2.5.0 : [lib strip 옵션 지원](https://github.com/fatima-go/gofar/issues/22)
- v2.4.0 : [빌드시 git 정보에 repo 정보 추가](https://github.com/fatima-go/gofar/issues/20)
- v2.3.1 : [배포정보의 git 관련 정보 수정](https://github.com/fatima-go/gofar/issues/18)
- v2.3.0 : [CGO 크로스 컴파일 환경 지원](https://github.com/fatima-go/gofar/issues/15)
- v2.2.1 : [잘못된 플랫폼 정보로 컴파일하는 버그 수정](https://github.com/fatima-go/gofar/issues/13)
- v2.2.0 : [로컬 플랫폼 먼저 컴파일](https://github.com/fatima-go/gofar/issues/11)

# platforms for compiling

gofar 에서 패키징시에 바이너리의 경우 타겟이 되는 플랫폼은 $HOME/.fatima/gofar.yaml 파일에 명시한다<br>
해당 파일이 없을 경우 gofar 에서는 자동 생성해주며 사용자가 원할 경우 gofar.yaml 파일에 직접 플랫폼을 추가할 수 있다.

- gofar configuration yaml 파일 예제
```shell
$ cat ~/.fatima/gofar.yaml
---
# if you want to check platform support list, use below command
# $ go tool dist list
# 
platform_list:
  - os: linux
    arch: amd64
  - os: linux
    arch: arm64
  - os: darwin
    arch: arm64
```

만약 크로스 컴파일을 위해 별도의 CC 를 지정하려면 각 플랫폼별로 알맞는 컴파일러를 등록해 둔다

```yaml
platform_list:
  - os: linux
    arch: amd64
    cc: x86_64-pc-linux-gcc
  - os: linux
    arch: arm64
    cc: aarch64-unknown-linux-gnu-gcc
  - os: darwin
    arch: arm64
```

- gofar 로 빌드 후 far 파일 내부의 구조 예제
```powershell
$ ls -l
-rw-r--r--  1 dave  staff      1523  8  8 13:22 application.properties   <- process config 파일
-rw-r--r--  1 dave  staff       146  8  8 13:22 deployment.json   <- 빌드배포 정보 파일
-rw-r--r--  1 dave  staff         0  8  8 13:22 platform  <- 플랫폼 디렉토리

$ tree -d
.
└── platform
    ├── darwin_arm64
    ├── linux_amd64
    └── linux_arm64
    
$ ls -l platform/linux_arm64
-rwxr--r--  1 dave  staff  26359299  8  8 13:22 helloworld   <- 실행 파일

$ file platform/linux_arm64/helloworld
helloworld: ELF 64-bit LSB executable, ARM aarch64, version 1 (SYSV), statically linked, not stripped
```

# project config

`$HOME/.fatima/gofar.yaml` 이 빌드 플랫폼 같은 전역 설정이라면, 프로젝트 디렉토리의 `.gofar/` 하위 설정은 해당 프로젝트 전용 설정이다.<br>
`cmd` 하위에 바이너리가 여러개인 프로젝트에서 바이너리별로 빌드 제외, strip, 배포 환경(profile)별 설치 여부를 지정한다.<br>
설정 파일이 없으면 기존과 동일하게 동작한다.

`gofar process_name` 실행시 아래 순서로 설정 파일 1개를 사용한다 (병합하지 않는다).

1. `.gofar/<process_name>.yaml` : 해당 process(far) 전용 설정
2. `.gofar/gofar.yaml` : 공통 설정

```
.gofar/
  knowledgesvc.yaml   <- gofar knowledgesvc
  other-svc.yaml      <- gofar other-svc
  gofar.yaml          <- process 전용 설정이 없을 때 (선택)
```

한 프로젝트에 process 가 여러개라면 process 별 파일로 관리한다. 설정은 해당 process 기준으로 작성하며, 다른 process 의 바이너리는 보통 `build.exclude` 로 제외한다.<br>
process 별 파일이 있는데 실행한 process 의 파일이 없어 공통 설정을 쓰거나 설정 없이 빌드하는 경우 경고한다.

설정 파일은 공통 문서 1개와 profile 문서(`fatima.profile`) N개로 구성된 multi-doc yaml 이다.

```yaml
---
# 공통 문서 (fatima.profile 없음)
build:
  # 빌드도, far 포함도 하지 않는 바이너리
  exclude: [knowcli, guide-bundle, knowledge-verify]
  # none(기본) | tools(process 외 바이너리) | all. -s 옵션은 all 과 같다
  strip: tools
  # true 이면 -c 옵션 없이도 CGO 를 활성화한다 (기본 false). cgo 를 쓰는 프로젝트라면 지정해 둔다
  cgo: true
install:
  # far 에는 포함하지만 기본적으로 설치하지 않는 바이너리
  exclude: [knowledge-backfill, knowledge-scope-review]
---
# profile 문서 : 설치 정책만 덮어쓴다
fatima:
  profile: local
install:
  include: [knowledge-backfill, knowledge-scope-review]
```

- process 바이너리는 gofar 실행시 지정한 process_name 과 이름이 같은 바이너리이며, 나머지는 tool 로 취급한다 (해당 바이너리가 없으면 경고)
- 빌드는 profile 과 무관하게 1번만 수행하므로 `build` 는 공통 문서에만 둘 수 있다
- `build.cgo: true` 이면 `-c` 옵션을 준 것과 같다. 크로스 컴파일 대상 플랫폼에 `cc` 가 지정되어 있지 않으면 경고한다 (위의 platforms for compiling 참고)
- 다음의 경우 빌드를 중단한다
  - 존재하지 않는 바이너리 이름 또는 잘못된 값
  - process 바이너리를 빌드 또는 설치에서 제외
  - `build.exclude` 된 바이너리를 `install` 에 지정
  - 공통 문서가 2개 이상이거나 같은 profile 문서가 중복
- 알 수 없는 키는 상위 버전 gofar 의 설정일 수 있으므로 무시하고 경고한다. 오타일 수 있으니 경고를 확인한다
- `install.exclude` 했지만 어떤 profile 에서도 include 하지 않은 바이너리는 `build.exclude` 로 옮기라고 경고한다

설정이 있으면 `deployment.json` 에 바이너리별 정책(`binaries`)을 기록한다.<br>
fatima-cmd(rodeploy)는 jupiter 로그인 응답의 profile 을 보고 해당 profile 에 설치하지 않는 바이너리를 제외한 임시 far 를 만들어 배포한다. 원본 far 는 그대로 두므로 여러 환경에 반복해서 배포할 수 있다.

```json
"binaries": {
  "knowledgesvc":       {"role": "process"},
  "knowledge-query":    {"role": "tool", "strip": true},
  "knowledge-backfill": {"role": "tool", "strip": true, "install": {"default": false, "profiles": {"local": true}}}
}
```

`install` 이 없으면 모든 profile 에 설치하고, 있으면 `profiles[profile]` 값을, 해당 profile 이 없으면 `default` 값을 따른다.
