---
title: Install Nabu
params:
  readiness: production
  kind: presentation
---
Nabu is one file. Download the one for your laptop from the
[latest release][release]:

| Laptop | File |
|---|---|
| Linux (Intel or AMD) | `nabu-amd-lnx` |
| Linux (ARM) | `nabu-arm-lnx` |
| Mac (Apple silicon: M1 and later) | `nabu-arm-mac` |
| Mac (Intel) | `nabu-amd-mac` |
| Windows (Intel or AMD) | `nabu-amd-win.exe` |
| Windows (ARM) | `nabu-arm-win.exe` |

Rename it to `nabu` (`nabu.exe` on Windows) and put it in a folder on your
`PATH`. On Linux or a Mac:

```bash
chmod +x nabu-arm-mac
sudo mv nabu-arm-mac /usr/local/bin/nabu
```

If you already use [clog][clog], one command does the same and also checks the
download against the release's checksums:

```bash
clog Install nabu
```

Check it works:

```bash
nabu --version
```

**A Mac says it cannot check the file?** Run `xattr -d com.apple.quarantine
/usr/local/bin/nabu` once. **Windows SmartScreen warns you?** Choose
*More info*, then *Run anyway*.

[release]: https://github.com/mrmxf/nabu/releases/latest
[clog]: https://github.com/mrmxf/clog
