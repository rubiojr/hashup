# Running HashUp

1. Install HashUp and command line tools:

```bash
go install github.com/rubiojr/hashup@latest
go install github.com/rubiojr/hashup/cmd/hs@latest
```

2. Setup a HashUp server node

```bash
hashup setup
```


3. Start the NATS server

```bash
hashup nats
```

4. Scan a directory

```bash
hashup scan ~/Documents # Scan and queue the scanned files to be indexed

# Requeue unchanged files when rebuilding an index
hashup scan --force ~/Documents

# Scan immediately, then repeat every hour
hashup scan --every 1h ~/Documents

# This can run in parallel
hashup store
```

5. Search indexed files

Use the CLI to search for file names.

```
hs search test
```

To search a remote HashUp API by default, create `~/.config/hashup/hs.toml`:

```toml
[main]
api_server_url = "https://hashup.example.com"
```

The `--server-url` flag and `HASHUP_API_URL` environment variable override the
configured URL. Use `hs --config /path/to/hs.toml search test` to load another
configuration file.

6. Download and install the HashUp App

Get it from https://github.com/rubiojr/hashup-app
