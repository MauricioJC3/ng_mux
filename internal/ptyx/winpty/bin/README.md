# Vendored winpty binaries

`winpty.dll` and `winpty-agent.exe` are the **x64** builds from
[rprichard/winpty](https://github.com/rprichard/winpty) release **0.4.3**
(`winpty-0.4.3-msvc2015.zip`). They are embedded into the ngmux executable
(`//go:embed`) and unpacked at runtime only on Windows builds that lack ConPTY
(Windows Server 2016 and earlier).

winpty is MIT-licensed; see `LICENSE` in this directory.

SHA-256 of the vendored files:

```
9add1a61155ec47cf6f347faf776b746eebbde1dc9360d81b8a909da34650642  winpty-agent.exe
936f611c2129600d35ab7aad45546a837f4f3a9ca7f673e5d66b48c313b9cd75  winpty.dll
```

To update: download a newer winpty release, replace both files with the `x64`
build, refresh the hashes above, and run the tests on Windows.
