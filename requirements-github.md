# Github workflow requirements

I want a github work which runs when a tag starting with a 'v' is pushed (for example v1.0.0). The following should be done:
- all the build target are build, so linux, windows, mac, on all the relevant architectures
  - use the `Makefile` for building, the desied GOOS/GOARCH combinations are set there.
  - `make build-all` build all necessary targets
  - build artifacts are located in ./build
- a release is created, the version number is derived from the tag, removing the 'v' prefix.
- all resulting binaries are put in the release (do you call this an artifact or a release binary?)
- if a tag has a `-rc*` postfix, it's a pre-release 
- a docker build is kicked off 
- the Dockerfile is in the root
- two containers are built (assume we are creating 1.0.0):
  - devbfvio/astro-archiver:1.0.0 (the x64 arch)
  - ~~devbfvio/astro-archiver:1.0.0-arm (the ARM arch)~~ 
  - for now just `linux/amd64`
  - the version we're build is always also `:latest`
  - these are the linux images
- DOCKERHUB_USERNAME is a var
- DOCKERHUB_TOKEN is a secret
- auto generate release notes
  
An ls of the `build/` directory after a `make build-all` looks like:
```
total 127248
-rw-r--r-- 1 bronco 197121 19124128 Jan 28 09:17 astro-ai-archiver-darwin-amd64
-rw-r--r-- 1 bronco 197121 18290466 Jan 28 09:18 astro-ai-archiver-darwin-arm64
-rw-r--r-- 1 bronco 197121 18674961 Jan 28 09:17 astro-ai-archiver-linux-amd64
-rw-r--r-- 1 bronco 197121 17748514 Jan 28 09:18 astro-ai-archiver-linux-arm64
-rwxr-xr-x 1 bronco 197121 19229696 Jan 28 09:16 astro-ai-archiver-windows-amd64.exe
-rwxr-xr-x 1 bronco 197121 17993216 Jan 28 09:17 astro-ai-archiver-windows-arm64.exe
```
