package game

import "fmt"

const (
	CheckStatAttack        = "attack"
	CheckStatDefense       = "defense"
	CheckStatSpeed         = "speed"
	CheckStatComprehension = "comprehension"
	CheckStatLuck          = "luck"
)

var checkStats = map[string]bool{
	CheckStatAttack:        true,
	CheckStatDefense:       true,
	CheckStatSpeed:         true,
	CheckStatComprehension: true,
	CheckStatLuck:          true,
}

type CheckRequest struct {
	Stat       string `json:"stat"`
	Difficulty int    `json:"difficulty"`
}

type CheckResult struct {
	Success   bool `json:"success"`
	Roll      int  `json:"roll"`
	Threshold int  `json:"threshold"`
	Margin    int  `json:"margin"`
}

// ResolveCheck runs a deterministic probability check against player stats.
// Callers supply roll (1-100) so outcomes stay reproducible; the LLM never
// picks the roll itself.
func ResolveCheck(save SaveGame, req CheckRequest, roll int) (CheckResult, error) {
	if !checkStats[req.Stat] {
		return CheckResult{}, fmt.Errorf("unknown check stat %q", req.Stat)
	}
	if roll < 1 || roll > 100 {
		return CheckResult{}, fmt.Errorf("roll out of range: %d", roll)
	}
	statValue := statByName(save.Player.Stats, req.Stat)
	threshold := clamp(50+(statValue-5)*5-req.Difficulty, 5, 95)
	return CheckResult{
		Success:   roll <= threshold,
		Roll:      roll,
		Threshold: threshold,
		Margin:    threshold - roll,
	}, nil
}

func statByName(stats Stats, name string) int {
	switch name {
	case CheckStatAttack:
		return stats.Attack
	case CheckStatDefense:
		return stats.Defense
	case CheckStatSpeed:
		return stats.Speed
	case CheckStatComprehension:
		return stats.Comprehension
	case CheckStatLuck:
		return stats.Luck
	default:
		return 0
	}
}
