package config

import (
	"math/rand/v2"
	"time"
)

// Verbs are the words that follow "Golem" on the working line
// ("Golem walking…"). They rotate every VerbEvery while the agent works.
var Verbs = []string{
	"walking",
	"figuring out",
	"lumbering",
	"stomping about",
	"digging through",
	"sifting",
	"kneading the clay",
	"carving",
	"chiseling",
	"rummaging",
	"pondering",
	"puzzling it out",
	"grinding",
	"shuffling",
	"tracing",
	"piecing together",
	"mulling it over",
	"rolling along",
	"sniffing around",
	"assembling",
	"untangling",
	"hauling stones",
	"thinking it through",
	"trudging",
}

// VerbEvery is how long each verb stays on screen.
const VerbEvery = 3 * time.Second

// NewVerbSeed picks a random starting verb so turns do not all open with
// the same word.
func NewVerbSeed() int { return rand.IntN(len(Verbs)) }

// VerbAt returns the verb for a turn that started with seed and has been
// running for elapsed.
func VerbAt(seed int, elapsed time.Duration) string {
	step := int(elapsed / VerbEvery)
	// A stride coprime-ish with the list length keeps neighbours unrelated.
	return Verbs[(seed+step*7)%len(Verbs)]
}
