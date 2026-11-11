package sky

// foresightState holds the plans the visualiser has been given, the most
// recent last, keyed by the name of the track as Step is given it.
type foresightState struct {
	plans []foreseen
}

type foreseen struct {
	track string
	plan  *Plan
}

// foresightKept is how many plans the visualiser holds at once.
const foresightKept = 4

// Foresee gives the visualiser the plan of a track, named as Step names it.
// A plan for a track it already holds replaces the one it had.
func (s *State) Foresee(track string, p *Plan) {
	f := &s.foresight
	kept := f.plans[:0]
	for _, old := range f.plans {
		if old.track != track {
			kept = append(kept, old)
		}
	}
	f.plans = append(kept, foreseen{track: track, plan: p})
	if len(f.plans) > foresightKept {
		f.plans = f.plans[1:]
	}
}

// plan returns the plan of the track playing, or nil when there is none.
func (s *State) plan() *Plan {
	for _, f := range s.foresight.plans {
		if f.track == s.track {
			return f.plan
		}
	}
	return nil
}
