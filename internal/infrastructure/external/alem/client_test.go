package alem

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBootcampDTO_Parsing(t *testing.T) {
	jsonData := `{
    "id": "7ed99bd0-87b2-4dbb-a97b-596c3f29c49b",
    "status": "COMPLETED",
    "start_at": "2025-06-16T04:00:00Z",
    "end_at": "2025-07-11T13:30:00Z",
    "title": "bootcamp-go",
    "type": "bootcamp",
    "total_xp": 39900,
    "user_xp": 19800,
    "children": [
        {
            "status": "COMPLETED",
            "start_at": "2025-06-16T04:00:00Z",
            "end_at": "2025-06-24T04:00:00Z",
            "index": 1,
            "title": "week01",
            "type": "week",
            "total_xp": 7800,
            "user_xp": 2450,
            "children": [
                {
                    "id": "9ca4322d-ebd5-4ffa-a340-56fe811bbab1",
                    "status": "COMPLETED",
                    "start_at": "2025-06-16T04:00:00Z",
                    "end_at": "0001-01-01T00:00:00Z",
                    "index": 1,
                    "title": "intro",
                    "type": "intro",
                    "total_xp": 100,
                    "user_xp": 100
                }
            ]
        }
    ]
}`

	var bootcamp BootcampDTO
	err := json.Unmarshal([]byte(jsonData), &bootcamp)
	assert.NoError(t, err)

	assert.Equal(t, "7ed99bd0-87b2-4dbb-a97b-596c3f29c49b", bootcamp.ID)
	assert.Equal(t, "bootcamp-go", bootcamp.Title)
	assert.Equal(t, 39900, bootcamp.TotalXP)
	assert.Equal(t, 19800, bootcamp.UserXP)
	assert.Len(t, bootcamp.Children, 1)

	week := bootcamp.Children[0]
	assert.Equal(t, "week01", week.Title)
	assert.Equal(t, 7800, week.TotalXP)
	assert.Equal(t, 2450, week.UserXP)

	assert.Len(t, week.Children, 1)
	intro := week.Children[0]
	assert.Equal(t, "intro", intro.Title)
	assert.Equal(t, 100, intro.UserXP)
}

func TestBootcampFlattening(t *testing.T) {
	// Construct a sample bootcamp DTO
	bootcamp := &BootcampDTO{
		ID:    "bootcamp1",
		Title: "Go Bootcamp",
		Children: []BootcampNodeDTO{
			{
				Title: "Week 1",
				Children: []BootcampNodeDTO{
					{
						ID:     "task1",
						Title:  "Intro",
						UserXP: 100,
						Status: "COMPLETED",
					},
					{
						ID:     "task2",
						Title:  "Story 1",
						UserXP: 0,
						Status: "AVAILABLE",
					},
				},
			},
		},
	}

	mapper := NewMapper()
	completions := mapper.FlattenBootcampToCompletions(bootcamp, "student1")

	assert.Len(t, completions, 1)
	assert.Equal(t, "task1", completions[0].TaskID)
	assert.Equal(t, "student1", completions[0].StudentID)
	assert.Equal(t, 100, completions[0].XPEarned)
	assert.Equal(t, "passed", completions[0].Status)
}
