# Vider

Vider (French word for "empty") is a tool that aims to finds the `node_modules` directories on your disk and deletes the ones you select, to free up space.

## Usage

Build the app and start it:

```sh
go build -o vider .
./vider
```

By default, the app scans your home directory, common project directories, and the current directory. You can give other directories on the command line:

```sh
./vider ~/Repositories
```

You can also set the `VIDER_ROOTS` variable. Separate the directories with `:`:

```sh
VIDER_ROOTS=~/code:~/work ./vider
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

## Why it is fast

The app scans and measures the directories in parallel:

- The scanner walks each root directory in its own goroutine. It does not enter `.git` directories, caches, and other directories that cannot contain a project.
- The scanner measures the size of a `node_modules` directory as soon as it finds the directory. The size scan runs in parallel with the walk. Results appear in the list while the scan continues.
- The app deletes the selected directories at the same time, up to 8 at once.

You can set the `VIDER_WORKERS` variable to change how many size scans run at the same time. The default is 32.
