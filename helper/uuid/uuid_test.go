package uuid

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerate(t *testing.T) {
	testCases := []struct {
		count int
		name  string
	}{
		{count: 100, name: "generate 100 unique uuids"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			original := Generate()

			for i := 0; i < 100; i++ {
				var previous string

				current := Generate()

				assert.NotEqual(t, current, original, tc.name)
				assert.NotEqual(t, current, previous, tc.name)

				matched, err := regexp.MatchString(
					"[\\da-f]{8}-[\\da-f]{4}-[\\da-f]{4}-[\\da-f]{4}-[\\da-f]{12}", current)

				assert.True(t, matched, tc.name)
				assert.Nil(t, err, tc.name)
			}
		})
	}
}
