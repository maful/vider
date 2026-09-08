# @vider-app/cli

Vider (French for "empty") finds the `node_modules` directories on your disk and deletes the ones you select, to free up space.

## Usage

```sh
npx @vider-app/cli
```

No installation is necessary. The first run downloads the engine for your platform. Later runs start it immediately.

Vider supports macOS on Apple Silicon and Intel, Linux x64 and ARM64, and Windows x64.

To install the `vider` command globally, run:

```sh
npm install --global @vider-app/cli
vider
```

By default, the app scans your home directory, common project directories, and the current directory. You can give other directories on the command line:

```sh
npx @vider-app/cli ~/Repositories
```

You can also set the `VIDER_ROOTS` variable. Separate the directories with `:`:

```sh
VIDER_ROOTS=~/code:~/work npx @vider-app/cli
```

## Keys

| Key | Action |
| --- | --- |
| `↑` / `↓` or `k` / `j` | Move the cursor |
| `space` | Select one directory, or unselect it |
| `a` | Select all directories, or unselect all |
| `d` or `enter` | Delete the selected directories (the app asks for `y` / `n`) |
| `r` | Scan again |
| `q` or `ctrl+c` | Close the app |

> CAUTION: The app deletes the directories that you select. You cannot get them back.

`VIDER_WORKERS` (default 32) caps how many size scans run at the same time.

## Develop the engine

The engine is Go and lives in the repository root. Run the Node shim against a locally built binary:

```sh
go build -o vider .
VIDER_BIN=./vider node packages/vider/bin.js
```

Run the tests:

```sh
cd packages/vider
npm install --omit=optional
npm test
```
