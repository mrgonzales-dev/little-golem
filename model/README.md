# model

This is where the model goes.

Put the GGUF files here. little-golem loads the first of these at startup and
switches between them with `ctrl+m` or `/models [name]`:

```
model/MiniCPM5-2B-Q4_K_M.gguf
model/Spark-X2.5-1.7B-Q4_K_M.gguf
```

The model files themselves are git-ignored (see `.gitignore`); only this README
is tracked. To add or change models, edit `Models` in `src/config/config.go`.
