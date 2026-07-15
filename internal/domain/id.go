package domain

import (
	"fmt"
	"strings"

	"github.com/oklog/ulid/v2"
)

func NewID(prefix string) string {
	return fmt.Sprintf("%s_%s", prefix, strings.ToLower(ulid.Make().String()))
}
