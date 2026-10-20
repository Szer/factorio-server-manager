package logging

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

func TestDebugLevel(t *testing.T) {
	var output bytes.Buffer
	old := log.Writer()
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(old); Configure("") })
	for _, level := range []string{"", "info", "debug"} {
		output.Reset()
		if err := Configure(level); err != nil {
			t.Fatal(err)
		}
		Debugf("file diagnostic %s", "present")
		log.Print("existing output")
		if strings.Contains(output.String(), "DEBUG file diagnostic") != (level == "debug") {
			t.Fatalf("unexpected debug output for %q", level)
		}
		if !strings.Contains(output.String(), "existing output") {
			t.Fatal("existing output suppressed")
		}
	}
	if Configure("typo") == nil {
		t.Fatal("invalid level accepted")
	}
}
