# Pragma Web UI

This React/Vite project is the development surface for the native web UI. The
production build is emitted into `../internal/web/static` and embedded by Go.

## Development

Start the Pragma web API on the Vite proxy's default target:

```sh
pragma --web-addr 127.0.0.1:4817
```

Then run the Vite dev server from this directory:

```sh
npm run dev
```

If the Pragma API is listening somewhere else, set `PRAGMA_WEB_API` before
starting Vite:

```sh
PRAGMA_WEB_API=http://127.0.0.1:49152 npm run dev
```

## Build And Test

```sh
npm test
npm run build
```
