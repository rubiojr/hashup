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

4. Index a directory

```bash
hashup index ~/Documents # Scan and queue the scanned files to be indexed

# This can run in parallel
hashup store
```

5. Search indexed files

Use the CLI to search for file names.

```
hs search test
```

6. Download and install the HashUp App

Get it from https://github.com/rubiojr/hashup-app
