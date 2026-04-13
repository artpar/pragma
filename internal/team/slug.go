package team

import (
	"crypto/rand"
	"math/big"
)

// Word lists adapted from the TS reference (words.ts).
// Format: "adjective-verb-noun" for unique team names.

var adjectives = []string{
	"abstract", "agile", "ancient", "bold", "bright", "cached", "calm",
	"clever", "cosmic", "crystal", "curious", "daring", "deep", "eager",
	"electric", "eloquent", "eternal", "fierce", "fluent", "gentle",
	"golden", "greedy", "hidden", "honest", "ionic", "keen", "lazy",
	"light", "lively", "lucid", "magic", "mellow", "mighty", "mystic",
	"noble", "novel", "opal", "parallel", "patient", "polite", "prime",
	"proud", "pure", "quick", "quiet", "rapid", "robust", "rustic",
	"sacred", "serene", "sharp", "silent", "smooth", "solar", "sonic",
	"stable", "steady", "subtle", "swift", "tidal", "tranquil", "ultra",
	"vast", "velvet", "vivid", "warm", "wild", "wise", "young", "zen",
}

var verbs = []string{
	"blazing", "brewing", "building", "carving", "casting", "chasing",
	"climbing", "composing", "crafting", "crossing", "dancing", "dashing",
	"dreaming", "drifting", "forging", "gliding", "growing", "hunting",
	"jumping", "learning", "mapping", "merging", "mining", "molding",
	"parsing", "paving", "planting", "pondering", "questing", "racing",
	"reading", "roaming", "running", "sailing", "searching", "seeking",
	"shaping", "singing", "skating", "soaring", "solving", "sparking",
	"spinning", "sprinting", "streaming", "surfing", "testing", "thinking",
	"tracing", "trading", "training", "walking", "watching", "weaving",
	"winding", "writing",
}

var nouns = []string{
	"algorithm", "anchor", "atlas", "aurora", "beacon", "bridge",
	"canyon", "catalyst", "cedar", "cipher", "cloud", "comet",
	"compass", "condor", "coral", "crystal", "delta", "eagle",
	"echo", "ember", "falcon", "flame", "forest", "fountain",
	"galaxy", "glacier", "harbor", "hawk", "helix", "horizon",
	"island", "jade", "kernel", "lantern", "lattice", "lighthouse",
	"maple", "meadow", "mesa", "nebula", "nexus", "oasis",
	"ocean", "oracle", "orchid", "osprey", "panda", "pearl",
	"phoenix", "pine", "prism", "quartz", "raven", "reef",
	"river", "rocket", "sequoia", "sierra", "signal", "spark",
	"summit", "temple", "thunder", "tiger", "tower", "valley",
	"vertex", "violet", "vortex", "wave", "willow", "zenith",
}

// GenerateWordSlug returns a random "adjective-verb-noun" slug using crypto/rand.
func GenerateWordSlug() string {
	a := adjectives[cryptoRandInt(len(adjectives))]
	v := verbs[cryptoRandInt(len(verbs))]
	n := nouns[cryptoRandInt(len(nouns))]
	return a + "-" + v + "-" + n
}

func cryptoRandInt(max int) int {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		// Fallback: should never happen with crypto/rand
		return 0
	}
	return int(n.Int64())
}
