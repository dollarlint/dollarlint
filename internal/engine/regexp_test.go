package engine

import "testing"

func TestECMARegexp(t *testing.T) {
	re, err := compileECMARegexp(`^\p{L}+$`)
	if err != nil {
		t.Fatalf("compileECMARegexp: %v", err)
	}
	if !re.MatchString("schema") {
		t.Fatalf("expected regex to match letters")
	}
	if re.MatchString("schema-123") {
		t.Fatalf("expected regex not to match non-letters")
	}
	if re.String() != `^\p{L}+$` {
		t.Fatalf("regex string = %q", re.String())
	}
	if _, err := compileECMARegexp(`[`); err == nil {
		t.Fatalf("expected invalid regex error")
	}
}
