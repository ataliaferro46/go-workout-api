// Command demo generates a sample workout plan and prints it to stdout — a
// quick way to see the engine's output without running the HTTP server.
//
//	go run ./cmd/demo
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/ataliaferro46/go-workout-api/internal/domain"
	"github.com/ataliaferro46/go-workout-api/internal/exercise"
	"github.com/ataliaferro46/go-workout-api/internal/plan"
)

func main() {
	req := domain.GenerateRequest{
		Goal:           domain.GoalMuscleGain,
		Experience:     domain.Intermediate,
		DaysPerWeek:    4,
		SessionMinutes: 60,
		AvailableEquipment: []domain.Equipment{
			domain.Barbell, domain.Dumbbell, domain.Cable, domain.Machine,
			domain.PullupBar, domain.Bench,
		},
		Injuries: []domain.BodyPart{domain.LowerBack},
	}

	gen := plan.NewGenerator(exercise.Library(), time.Now().UnixNano())
	p, err := gen.Generate(req)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	printPlan(req, p)
}

func printPlan(req domain.GenerateRequest, p domain.WorkoutPlan) {
	fmt.Printf("Goal: %s    Experience: %s    Split: %s    Days/week: %d\n",
		p.Goal, p.Experience, p.Split, p.DaysPerWeek)
	if len(req.Injuries) > 0 {
		fmt.Printf("Working around injuries: %v\n", req.Injuries)
	}
	fmt.Println()

	for _, day := range p.Days {
		fmt.Printf("Day %d — %s\n", day.Index, day.Name)
		for _, pe := range day.Exercises {
			fmt.Printf("  %d. %-28s %d x %d-%d   rest %ds\n",
				pe.Order, pe.Exercise.Name, pe.Sets, pe.RepsLow, pe.RepsHigh, pe.RestSeconds)
		}
		fmt.Println()
	}
	for _, warn := range p.Warnings {
		fmt.Println("note:", warn)
	}
}
