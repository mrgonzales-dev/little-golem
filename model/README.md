# model

This is where the model goes.

Put the GGUF file here. little-golem loads:

```
model/MiniCPM5-2B-Q4_K_M.gguf
```

The model file itself is git-ignored (see `.gitignore`); only this README is
tracked. To use a different file, change `ModelPath` in `src/config/config.go`.
