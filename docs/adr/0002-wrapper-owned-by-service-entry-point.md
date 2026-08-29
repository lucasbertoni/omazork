# The Wrapper is owned by a service entry point, not the Console overlay

The bar icon needs live "recap waiting" and "wrapper alive" state, but the shell gives plugins no shared-state store, and the Go wrapper Process historically lived inside the overlay's `Main.qml`. We split the manifest into `kinds: ["overlay", "bar-widget", "service"]` and moved the wrapper Process plus all NDJSON plumbing into `Service.qml` — created once at shell start, injected into the overlay as `service`, and reached from the bar widget via `bar.shell.serviceFor("omazork")`. This is the host's first-party pattern (the bundled media plugin works this way) and gives every bar instance live QML bindings with no polling, files, or IPC.

## Considered Options

- **Widget reads the overlay item via `bar.shell.panelLoaders["omazork"].item`**: minimal change, but couples to a private shell map and to `keepLoaded: true` never being dropped — breaks silently on a shell update.
- **Wrapper writes a status file the widget watches**: the wrapper only rewrites playthrough state on turns, has no "current game" pointer on disk, and can't report its own death.

## Consequences

- The service is instantiated synchronously for every enabled plugin at shell startup, so `Service.qml` must stay cheap: the Go process is spawned deferred (shortly after startup), not in `Component.onCompleted` work.
- Maturation events (`matured` over NDJSON, the maturation-check timer) now live in the service and no longer depend on the overlay being loaded — previously they would have died with the overlay if `keepLoaded` were ever dropped.
- The overlay keeps `keepLoaded: true` for now; dropping it (loading the Console on summon, re-attaching to the service's transcript) is a deliberate later change, not part of the bar-icon work.
