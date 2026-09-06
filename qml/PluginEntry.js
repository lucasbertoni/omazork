// Per-plugin settings on this plugin's entry in the host's shell.json. The
// entry lives in `plugins`, or — since the bar icon — in `bar.layout` (an
// entry there also enables the plugin); a stale copy in both is possible.
// `omarchy bar set` writes only the layout entry, so layout wins, and a key
// missing there still falls through to the `plugins` entry.
.pragma library

function setting(cfg, pluginId, key) {
    if (!cfg) return undefined
    var pools = []
    var layout = cfg.bar && cfg.bar.layout ? cfg.bar.layout : {}
    var sections = ["left", "center", "right"]
    for (var s = 0; s < sections.length; s++)
        if (layout[sections[s]]) pools.push(layout[sections[s]])
    pools.push(cfg.plugins || [])
    for (var p = 0; p < pools.length; p++)
        for (var i = 0; i < pools[p].length; i++) {
            var e = pools[p][i]
            if (e && e.id === pluginId && e[key] !== undefined) return e[key]
        }
    return undefined
}
