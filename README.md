# parg

parg is a parallel git operations tool.

## Installation

```bash
go install github.com/JackKCWong/par-git/bin/parg@latest
```

## Authentication

You only need credentials when parg talks to a remote server: listing repos
with `clone --org`, and fetching code with `clone` and `pull`. `grep` and
`stat` only read local repos and never need authentication.

### Listing org repos (`clone --org`)

Set the `GITHUB_TOKEN` environment variable to a GitHub personal access token
before running `clone --org`. Without a token you are limited to 60 API
requests per hour; with one, the limit rises to 5000, which matters for large
orgs. The same token works with GitHub Enterprise Server.

Keep in mind the token is only used to discover which repos exist in the org.
Actually cloning them needs the access described below, so `GITHUB_TOKEN`
alone won't let you clone private repos.

### Cloning and pulling

- **Public repos** need no setup — plain HTTPS URLs just work.
- **Private repos over HTTPS**: put URLs with your username and a personal
  access token in the file you pass to `clone -f`, one per line:

  ```
  https://<user>:<token>@github.com/myorg/myrepo.git
  ```

  parg does not use git's credential helpers, so the credentials must be in
  the URL itself.
- **Private repos over SSH**: use `git@github.com:myorg/myrepo.git`-style URLs.
  Make sure your SSH key is loaded in the agent (check with `ssh-add -l`, add
  with `ssh-add`).

`pull` fetches from whatever remotes are already configured in each repo, so
each repo needs its own working credentials as above.

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

Set `GITHUB_TOKEN` to raise the API rate limit from 60/hr to
5000/hr (see [Authentication](#authentication)).

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
