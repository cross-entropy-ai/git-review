package diff

import "testing"

func TestParseAPIPatch(t *testing.T) {
	h, err := ParseHunks("@@ -2,2 +2,2 @@ fn\n same\n-old\n+new\n@@ -9,0 +10 @@\n+last\n\\ No newline at end of file")
	if err != nil || len(h) != 2 || h[0].Lines[2].New != 3 || h[1].Lines[0].New != 10 {
		t.Fatalf("wrong parsed hunks: %+v %v", h, err)
	}
	for _, patch := range []string{"", "@@ -1 +1 @@\n-old", "@@ -0,0 +1 @@\n+a\n+b", "@@ -0,0 +1,2 @@\n+a\n@@ -0,0 +5 @@\n+b", "@@ -0,0 +1 @@\n?invalid", "@@ -999999999999999999999999 +1 @@\n+x"} {
		if _, err := ParseHunks(patch); err == nil {
			t.Fatalf("accepted incomplete patch %q", patch)
		}
	}
}
