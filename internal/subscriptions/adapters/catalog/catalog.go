package catalog

import (
	"bufio"
	_ "embed"
	"fmt"
	"strconv"
	"strings"

	"outages-bot/internal/subscriptions/application/core/domain"
)

//go:embed streets.tsv
var streetData string

func Load() ([]domain.Street, error) {
	var streets []domain.Street
	seen := make(map[int32]bool)
	scanner := bufio.NewScanner(strings.NewReader(streetData))
	for scanner.Scan() {
		fields := strings.SplitN(scanner.Text(), "\t", 2)
		if len(fields) != 2 {
			return nil, fmt.Errorf("invalid bundled street row")
		}
		id, err := strconv.ParseInt(fields[0], 10, 32)
		if err != nil || id <= 0 || strings.TrimSpace(fields[1]) == "" {
			return nil, fmt.Errorf("invalid bundled street")
		}
		if seen[int32(id)] {
			return nil, fmt.Errorf("duplicate bundled street ID %d", id)
		}
		seen[int32(id)] = true
		streets = append(streets, domain.Street{ID: int32(id), Name: fields[1]})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read bundled streets: %w", err)
	}
	return streets, nil
}
