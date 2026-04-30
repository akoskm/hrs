package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestProjectAddAndList(t *testing.T) {
	resetProjectCommandState()
	dbPath = t.TempDir() + "/hrs.db"
	fixturesPath = ""

	clientOut := &bytes.Buffer{}
	rootCmd.SetOut(clientOut)
	rootCmd.SetErr(clientOut)
	rootCmd.SetArgs([]string{"--db", dbPath, "client", "add", "Delta Labs"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("client add execute error = %v", err)
	}

	addOut := &bytes.Buffer{}
	rootCmd.SetOut(addOut)
	rootCmd.SetErr(addOut)
	rootCmd.SetArgs([]string{"--db", dbPath, "project", "add", "Elaiia", "--code", "elaiia", "--rate", "15000", "--currency", "CHF", "--client", "Delta Labs"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("project add execute error = %v", err)
	}
	if !strings.Contains(addOut.String(), "Elaiia\telaiia\t15000\tCHF\tDelta Labs") {
		t.Fatalf("project add output = %q", addOut.String())
	}

	updateOut := &bytes.Buffer{}
	rootCmd.SetOut(updateOut)
	rootCmd.SetErr(updateOut)
	rootCmd.SetArgs([]string{"--db", dbPath, "project", "update", "elaiia", "--billable=false"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("project update execute error = %v", err)
	}
	if !strings.Contains(updateOut.String(), "Elaiia\telaiia\t15000\tCHF\tfalse") {
		t.Fatalf("project update output = %q", updateOut.String())
	}

	listOut := &bytes.Buffer{}
	rootCmd.SetOut(listOut)
	rootCmd.SetErr(listOut)
	rootCmd.SetArgs([]string{"--db", dbPath, "project", "list"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("project list execute error = %v", err)
	}
	if !strings.Contains(listOut.String(), "Elaiia\telaiia\t15000\tCHF\tfalse\tDelta Labs") {
		t.Fatalf("project list output = %q", listOut.String())
	}
}

func TestProjectAddDefaultsCodeToSlugAndUpdateByName(t *testing.T) {
	resetProjectCommandState()
	dbPath = t.TempDir() + "/hrs.db"
	fixturesPath = ""

	addOut := &bytes.Buffer{}
	rootCmd.SetOut(addOut)
	rootCmd.SetErr(addOut)
	rootCmd.SetArgs([]string{"--db", dbPath, "project", "add", "Delta Labs AG"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("project add execute error = %v", err)
	}
	if !strings.Contains(addOut.String(), "Delta Labs AG\tdelta-labs-ag\t0\tEUR") {
		t.Fatalf("project add output = %q", addOut.String())
	}

	updateOut := &bytes.Buffer{}
	rootCmd.SetOut(updateOut)
	rootCmd.SetErr(updateOut)
	rootCmd.SetArgs([]string{"--db", dbPath, "project", "update", "Delta Labs AG", "--billable=false"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("project update execute error = %v", err)
	}
	if !strings.Contains(updateOut.String(), "Delta Labs AG\tdelta-labs-ag\t0\tEUR\tfalse") {
		t.Fatalf("project update output = %q", updateOut.String())
	}
}

func TestProjectTimeOffAllowanceSetListAndClear(t *testing.T) {
	resetProjectCommandState()
	dbPath = t.TempDir() + "/hrs.db"
	fixturesPath = ""

	addOut := &bytes.Buffer{}
	rootCmd.SetOut(addOut)
	rootCmd.SetErr(addOut)
	rootCmd.SetArgs([]string{"--db", dbPath, "project", "add", "hrs", "--code", "hrs"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("project add execute error = %v", err)
	}

	setOut := &bytes.Buffer{}
	rootCmd.SetOut(setOut)
	rootCmd.SetErr(setOut)
	rootCmd.SetArgs([]string{"--db", dbPath, "project", "time-off", "allowance", "set", "hrs", "Vacation", "2026", "20"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("allowance set execute error = %v", err)
	}
	if !strings.Contains(setOut.String(), "hrs\tVacation\t2026\t20") {
		t.Fatalf("allowance set output = %q", setOut.String())
	}

	listOut := &bytes.Buffer{}
	rootCmd.SetOut(listOut)
	rootCmd.SetErr(listOut)
	rootCmd.SetArgs([]string{"--db", dbPath, "project", "time-off", "allowance", "list", "hrs", "2026"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("allowance list execute error = %v", err)
	}
	if !strings.Contains(listOut.String(), "hrs\tVacation\t2026\t20\t0\t20") {
		t.Fatalf("allowance list output = %q", listOut.String())
	}

	clearOut := &bytes.Buffer{}
	rootCmd.SetOut(clearOut)
	rootCmd.SetErr(clearOut)
	rootCmd.SetArgs([]string{"--db", dbPath, "project", "time-off", "allowance", "clear", "hrs", "Vacation", "2026"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("allowance clear execute error = %v", err)
	}
	if !strings.Contains(clearOut.String(), "hrs\tVacation\t2026\tcleared") {
		t.Fatalf("allowance clear output = %q", clearOut.String())
	}

	listAfterClearOut := &bytes.Buffer{}
	rootCmd.SetOut(listAfterClearOut)
	rootCmd.SetErr(listAfterClearOut)
	rootCmd.SetArgs([]string{"--db", dbPath, "project", "time-off", "allowance", "list", "hrs", "2026"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("allowance list after clear execute error = %v", err)
	}
	if strings.TrimSpace(listAfterClearOut.String()) != "" {
		t.Fatalf("allowance list after clear output = %q, want empty", listAfterClearOut.String())
	}
}

func resetProjectCommandState() {
	projectAddClient = ""
	projectAddCode = ""
	projectAddRate = 0
	projectAddCurrency = "EUR"
	projectUpdateBillable = true
}
