package pagination

import (
	"fmt"
	"net/url"
	"strconv"

	"github.com/VanceMichael/greengrid/internal/domain"
)

type Page struct{ Limit, Offset int }
type Meta struct{ Total, Limit, Offset int }
type Result[T any] struct {
	Items []T
	Meta  Meta
}

func Default() Page { return Page{Limit: 25, Offset: 0} }
func Parse(values url.Values) (Page, error) {
	p := Default()
	if raw := values.Get("limit"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 1 || v > 100 {
			return Page{}, fmt.Errorf("%w: limit", domain.ErrInvalid)
		}
		p.Limit = v
	}
	if raw := values.Get("offset"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 0 {
			return Page{}, fmt.Errorf("%w: offset", domain.ErrInvalid)
		}
		p.Offset = v
	}
	return p, nil
}
func (p Page) Validate() error {
	if p.Limit < 1 || p.Limit > 100 || p.Offset < 0 {
		return fmt.Errorf("%w: page", domain.ErrInvalid)
	}
	return nil
}
func (m Meta) HasNext() bool { return m.Offset+m.Limit < m.Total }
func (m Meta) NextOffset() int {
	if !m.HasNext() {
		return m.Offset
	}
	return m.Offset + m.Limit
}
