# llama-server-setup

This is where the llama.cpp server build goes.

Copy the contents of your llama.cpp `build/bin/` directory here, so that
these exist:

```
llama-server-setup/llama-server
llama-server-setup/libggml-*.so*
llama-server-setup/libllama*.so*
```

little-golem starts `llama-server-setup/llama-server` on a free local port
and sets `LD_LIBRARY_PATH` to this folder for the shared libraries.

The binaries are git-ignored (see `.gitignore`); only this README is
tracked. To use another build, change `LlamaBin` and `LlamaLibDir` in
`src/config/config.go`.
