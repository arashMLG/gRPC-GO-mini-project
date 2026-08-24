package domain

import "strings"

// StatusWord is the input that asks for the player's current score rather
// than scoring a guess.
const StatusWord = "status"

// ScoreWord is the game's scoring rule: pure logic with no database, no
// network, and no clock. Because it is a plain function over plain values it
// can be tested exhaustively without any infrastructure at all, which is why
// it lives in the domain rather than inside the service or the gRPC handler.
func ScoreWord(word string) (delta int32, message string) {
	switch strings.ToLower(strings.TrimSpace(word)) {
	case "cat":
		return 1, "Good human"
	case "dog":
		return -1, "Bad human"
	case "":
		return 0, "Silent human"
	case "cog", "dat":
		return 0, "what?"
	default:
		return 0, "Unintelligible human"
	}
}

// IsStatusWord reports whether the input is the "tell me my score" command.
func IsStatusWord(word string) bool {
	return strings.ToLower(strings.TrimSpace(word)) == StatusWord
}
