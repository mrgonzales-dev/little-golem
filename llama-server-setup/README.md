# llama-server-setup

This is where the llama.cpp server build goes.

After setup, these should exist:

```
llama-server-setup/llama-server
llama-server-setup/libggml-*.so*
llama-server-setup/libllama*.so*
```

## Download a prebuilt release (Linux x86_64, CPU)

From the repo root:

```sh
TAG=b11411   # pick a release from https://github.com/ggml-org/llama.cpp/releases
curl -L "https://github.com/ggml-org/llama.cpp/releases/download/$TAG/llama-$TAG-bin-ubuntu-x64.tar.gz" \
  | tar xz --strip-components=1 -C llama-server-setup
```

Other platforms/backends (Vulkan, CUDA, ROCm, arm64, ...) are on the same
releases page; use the matching `llama-$TAG-bin-*.tar.gz`.

## Or build from source

```sh
git clone https://github.com/ggml-org/llama.cpp && cd llama.cpp
cmake -B build && cmake --build build --config Release -j
cp -a build/bin/. /path/to/little-golem/llama-server-setup/
```

## Notes

little-golem starts `llama-server-setup/llama-server` on a free local port
and sets `LD_LIBRARY_PATH` to this folder for the shared libraries.

The binaries are git-ignored (see `.gitignore`); only this README is
tracked. To use another build, change `LlamaBin` and `LlamaLibDir` in
`src/config/config.go`.
