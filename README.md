# gofar

gofar 는 golang 프로그램을 fatima 환경에 배포하기 위해 빌드를 도와주는 툴이다.

# Release

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
