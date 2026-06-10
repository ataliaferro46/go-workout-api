package exercise

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ataliaferro46/go-workout-api/internal/domain"
)

// externalSeedURL is the canonical Free Exercise DB (yuhonas/Everkinetic),
// public-domain, ~870 entries with full muscle/equipment/level tagging.
// We fetch it once on first boot (when the local foods table is sparse)
// and merge mapped entries into the seed. After that the table is large
// enough that SeedIfEmpty's "insert missing" path skips re-fetching.
const externalSeedURL = "https://raw.githubusercontent.com/yuhonas/free-exercise-db/main/dist/exercises.json"

// externalRecord mirrors one entry from the Free Exercise DB JSON.
type externalRecord struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Force            string   `json:"force"`     // "push" | "pull" | "static" | ""
	Level            string   `json:"level"`     // "beginner" | "intermediate" | "expert"
	Mechanic         string   `json:"mechanic"`  // "compound" | "isolation" | ""
	Equipment        string   `json:"equipment"` // "barbell", "dumbbell", "body only", etc.
	PrimaryMuscles   []string `json:"primaryMuscles"`
	SecondaryMuscles []string `json:"secondaryMuscles"`
	Category         string   `json:"category"` // "strength" | "cardio" | "stretching" | ...
}

// FetchExternalExercises pulls the Free Exercise DB and returns it as
// our domain.Exercise slice. Best-effort — returns nil + error on any
// network / parse failure so the boot path can continue with just the
// canonical seed.
func FetchExternalExercises(ctx context.Context) ([]domain.Exercise, error) {
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, externalSeedURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("external seed %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var recs []externalRecord
	if err := json.Unmarshal(body, &recs); err != nil {
		return nil, err
	}
	out := make([]domain.Exercise, 0, len(recs))
	for _, r := range recs {
		ex, ok := mapExternalRecord(r)
		if ok {
			out = append(out, ex)
		}
	}
	return out, nil
}

// mapExternalRecord projects one Free Exercise DB record onto our
// domain.Exercise. Returns ok=false when the record is unmappable
// (cardio / stretching / unknown muscle) so the caller skips it.
func mapExternalRecord(r externalRecord) (domain.Exercise, bool) {
	if r.Category != "strength" && r.Category != "powerlifting" && r.Category != "olympic weightlifting" {
		return domain.Exercise{}, false
	}
	primary, ok := mapMuscle(firstOr(r.PrimaryMuscles, ""))
	if !ok {
		return domain.Exercise{}, false
	}
	equipment, ok := mapEquipment(r.Equipment, r.Name)
	if !ok {
		return domain.Exercise{}, false
	}
	pattern := derivePattern(r, primary)
	compound := r.Mechanic == "compound"
	level := domain.Intermediate
	switch r.Level {
	case "beginner":
		level = domain.Beginner
	case "expert":
		level = domain.Advanced
	}
	// Convert ID to lowercase + dashes so it harmonizes with our existing
	// seed library naming convention.
	id := "ext-" + strings.ToLower(strings.ReplaceAll(r.ID, "_", "-"))
	secondary := make([]domain.MuscleGroup, 0, len(r.SecondaryMuscles))
	for _, sm := range r.SecondaryMuscles {
		if m, ok := mapMuscle(sm); ok && m != primary {
			secondary = append(secondary, m)
		}
	}
	return domain.Exercise{
		ID:                id,
		Name:              r.Name,
		PrimaryMuscle:     primary,
		SecondaryMuscles:  secondary,
		Pattern:           pattern,
		RequiredEquipment: equipment,
		Compound:          compound,
		MinLevel:          level,
	}, true
}

// mapMuscle maps the Free Exercise DB muscle taxonomy onto ours. The
// source has more granular labels (lats vs. middle back vs. lower back
// vs. traps); we collapse all of them to Back. Anything we can't map
// (forearms, neck, abductors, adductors) returns ok=false.
func mapMuscle(m string) (domain.MuscleGroup, bool) {
	switch strings.ToLower(strings.TrimSpace(m)) {
	case "chest":
		return domain.Chest, true
	case "biceps":
		return domain.Biceps, true
	case "triceps":
		return domain.Triceps, true
	case "shoulders":
		return domain.Shoulders, true
	case "lats", "middle back", "lower back", "traps":
		return domain.Back, true
	case "quadriceps", "quads":
		return domain.Quads, true
	case "hamstrings":
		return domain.Hamstrings, true
	case "glutes":
		return domain.Glutes, true
	case "calves":
		return domain.Calves, true
	case "abdominals", "core":
		return domain.Core, true
	}
	return "", false
}

// mapEquipment normalizes the Free Exercise DB equipment field into our
// equipment enum. "Bench" gets inferred from the exercise name (their
// schema doesn't have a bench tag).
func mapEquipment(e, name string) ([]domain.Equipment, bool) {
	lower := strings.ToLower(strings.TrimSpace(e))
	nameLower := strings.ToLower(name)
	addBench := strings.Contains(nameLower, "bench") || strings.Contains(nameLower, "incline") || strings.Contains(nameLower, "decline")

	var primary []domain.Equipment
	switch lower {
	case "barbell", "e-z curl bar":
		primary = []domain.Equipment{domain.Barbell}
	case "dumbbell":
		primary = []domain.Equipment{domain.Dumbbell}
	case "cable":
		primary = []domain.Equipment{domain.Cable}
	case "machine":
		primary = []domain.Equipment{domain.Machine}
	case "kettlebell", "kettlebells":
		primary = []domain.Equipment{domain.Kettlebell}
	case "bands", "band":
		primary = []domain.Equipment{domain.Bands}
	case "body only", "none", "":
		primary = []domain.Equipment{domain.Bodyweight}
		// If "pull-up" or "chin-up" in the name, add pull-up bar.
		if strings.Contains(nameLower, "pull-up") || strings.Contains(nameLower, "pull up") ||
			strings.Contains(nameLower, "chin-up") || strings.Contains(nameLower, "chin up") {
			primary = append(primary, domain.PullupBar)
		}
	default:
		return nil, false // exercise ball / foam roll / medicine ball / other
	}
	if addBench {
		primary = append(primary, domain.Bench)
	}
	return primary, true
}

// derivePattern figures out our movement pattern from name + force +
// muscle. Their schema doesn't carry a pattern field, so we infer.
func derivePattern(r externalRecord, primary domain.MuscleGroup) domain.MovementPattern {
	nameLower := strings.ToLower(r.Name)

	// Specific patterns first — these are unambiguous when the keyword appears.
	switch {
	case containsAnyKeyword(nameLower, "squat", "split squat", "lunge", "step-up", "step up"):
		if containsAnyKeyword(nameLower, "lunge", "step-up", "step up", "split squat") {
			return domain.LungePattern
		}
		return domain.SquatPattern
	case containsAnyKeyword(nameLower, "deadlift", "rdl", "romanian", "good morning", "hip thrust", "glute bridge", "kettlebell swing"):
		return domain.HingePattern
	case primary == domain.Core:
		return domain.CorePattern
	}

	// Compound mapped by force + primary
	if r.Mechanic == "compound" {
		switch primary {
		case domain.Chest:
			return domain.HorizontalPush
		case domain.Shoulders:
			return domain.VerticalPush
		case domain.Back:
			if containsAnyKeyword(nameLower, "pulldown", "pull-up", "pull up", "chin-up", "chin up", "pullover") {
				return domain.VerticalPull
			}
			return domain.HorizontalPull
		case domain.Quads:
			return domain.SquatPattern
		case domain.Hamstrings, domain.Glutes:
			return domain.HingePattern
		}
	}
	return domain.Isolation
}

func containsAnyKeyword(s string, keys ...string) bool {
	for _, k := range keys {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

func firstOr(xs []string, def string) string {
	if len(xs) == 0 {
		return def
	}
	return xs[0]
}
