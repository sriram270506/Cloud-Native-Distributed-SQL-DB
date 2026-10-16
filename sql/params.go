package sql

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

// literalMarker starts the body of a masked literal. The regex parser only ever sees
// '<marker><hex>', which holds no spaces, commas, quotes or keywords, so literal content
// can never be read as SQL syntax.
const literalMarker = "\x00"

// maskLiterals replaces every quoted literal with its hex-encoded form.
func maskLiterals(statement string) (string, error) {
	var out strings.Builder
	for i := 0; i < len(statement); i++ {
		if statement[i] != '\'' {
			out.WriteByte(statement[i])
			continue
		}
		var content strings.Builder
		closed := false
		for i++; i < len(statement); i++ {
			if statement[i] == '\'' {
				if i+1 < len(statement) && statement[i+1] == '\'' {
					content.WriteByte('\'')
					i++
					continue
				}
				closed = true
				break
			}
			content.WriteByte(statement[i])
		}
		if !closed {
			return "", fmt.Errorf("unterminated string literal")
		}
		out.WriteString("'" + literalMarker + hex.EncodeToString([]byte(content.String())) + "'")
	}
	return out.String(), nil
}

// Bind substitutes each ? placeholder outside string literals with a safely quoted value,
// so client input is always data and never SQL.
func Bind(statement string, params []interface{}) (string, error) {
	var out strings.Builder
	next, quoted := 0, false
	for i := 0; i < len(statement); i++ {
		char := statement[i]
		if char == '\'' {
			quoted = !quoted
		}
		if char != '?' || quoted {
			out.WriteByte(char)
			continue
		}
		if next >= len(params) {
			return "", fmt.Errorf("statement has more placeholders than the %d parameter(s) given", len(params))
		}
		literal, err := sqlLiteral(params[next])
		if err != nil {
			return "", fmt.Errorf("parameter %d: %w", next+1, err)
		}
		out.WriteString(literal)
		next++
	}
	if next != len(params) {
		return "", fmt.Errorf("statement has %d placeholder(s) but %d parameter(s) were given", next, len(params))
	}
	return out.String(), nil
}

func sqlLiteral(value interface{}) (string, error) {
	switch typed := value.(type) {
	case string:
		return "'" + strings.ReplaceAll(typed, "'", "''") + "'", nil
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64), nil
	case int:
		return strconv.Itoa(typed), nil
	case bool:
		return strconv.FormatBool(typed), nil
	default:
		return "", fmt.Errorf("unsupported type %T (use string, number or boolean)", value)
	}
}
