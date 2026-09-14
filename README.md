# parg

parg is a parallel git operations tool.

## Installation

```bash
go install github.com/JackKCWong/par-git/bin/parg@latest
```

## Commands

### clone

Clone multiple git repos in parallel.

Clone from a file containing URLs:

```bash
parg clone -f urls.txt -c 8
```

Or clone every repo in a GitHub org (or user) by URL. Repos are cloned into a
subdirectory named after the org:

```bash
parg clone --org https://github.com/hsbc -c 8
```

GitHub Enterprise hosts are auto-detected:

```bash
parg clone --org https://github.mycompany.com/team -c 8
```

Set `GITHUB_TOKEN` to raise the API rate limit from 60/hr (unauthenticated) to
5000/hr.

Flags:
- `-f, --file`: File containing git URLs to clone (one per line)
- `--org`: GitHub org or user URL; clones every repo under it
- `-c, --parallelism`: Number of clones to run in parallel (default: 8)
- `-b, --branch`: Branch to checkout after clone
- `--depth`: Create a shallow clone with the specified history depth (0 for full clone)
- `--recurse-submodules`: Initialize and clone submodules
- `--single-branch`: Clone only the specified branch
- `--bare`: Clone as a bare repository

`-f` and `--org` are mutually exclusive; exactly one must be provided.

### grep

Run git grep in parallel across all git repos in a directory.

```bash
parg grep -C <directory> <pattern>
```

Flags:
- `-C, --directory`: Root directory to search for git repos (default: current directory)

## License

MIT
