# parg

parg is a parallel git operations tool.

## Installation

```bash
go install github.com/JackKCWong/par-git/bin/parg@latest
```

## Commands

### clone

Clone multiple git repos in parallel from a file containing URLs.

```bash
parg clone -f urls.txt -c 8
```

Flags:
- `-f, --file`: File containing git URLs to clone (one per line)
- `-c, --parallelism`: Number of clones to run in parallel (default: 8)

### grep

Run git grep in parallel across all git repos in a directory.

```bash
parg grep -C <directory> <pattern>
```

Flags:
- `-C, --directory`: Root directory to search for git repos (default: current directory)

## License

MIT
