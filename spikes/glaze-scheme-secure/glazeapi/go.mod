module goleo-scheme-glazeapi

go 1.26

require github.com/crgimenes/glaze v0.0.31

require github.com/ebitengine/purego v0.10.1 // indirect

// Prove the proposed glaze scheme-handler API against the local fork copy.
replace github.com/crgimenes/glaze => github.com/daforester/glaze v0.0.32-goleo.3
