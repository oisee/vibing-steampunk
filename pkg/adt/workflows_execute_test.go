package adt

import "testing"

func TestAnalyzeExecuteABAPUnitResult(t *testing.T) {
	tests := []struct {
		name       string
		result     *UnitTestResult
		wantOK     bool
		wantOutput []string
		wantAlerts int
	}{
		{
			name: "intentional result assertion",
			result: unitResultWithMethodAlerts(UnitTestAlert{
				Kind: "failedAssertion", Title: "EXEC_RESULT:synthetic value",
			}),
			wantOK: true, wantOutput: []string{"synthetic value"}, wantAlerts: 1,
		},
		{
			name: "marker in assertion details",
			result: unitResultWithMethodAlerts(UnitTestAlert{
				Kind: "failedAssertion", Title: "Assertion failed", Details: []string{"EXEC_RESULT:detail value"},
			}),
			wantOK: true, wantOutput: []string{"detail value"}, wantAlerts: 1,
		},
		{
			name: "real failed assertion is not hidden by marker",
			result: unitResultWithMethodAlerts(
				UnitTestAlert{Kind: "failedAssertion", Title: "EXEC_RESULT:synthetic value"},
				UnitTestAlert{Kind: "failedAssertion", Title: "synthetic assertion failure"},
			),
			wantOutput: []string{"synthetic value"}, wantAlerts: 2,
		},
		{
			name: "exception is failure",
			result: unitResultWithMethodAlerts(
				UnitTestAlert{Kind: "failedAssertion", Title: "EXEC_RESULT:synthetic value"},
				UnitTestAlert{Kind: "exception", Title: "synthetic exception"},
			),
			wantOutput: []string{"synthetic value"}, wantAlerts: 2,
		},
		{
			name: "class alert is failure",
			result: &UnitTestResult{Classes: []UnitTestClass{{
				Alerts:      []UnitTestAlert{{Kind: "exception", Title: "synthetic class exception"}},
				TestMethods: []UnitTestMethod{{Alerts: []UnitTestAlert{{Kind: "failedAssertion", Title: "EXEC_RESULT:done"}}}},
			}}},
			wantOutput: []string{"done"}, wantAlerts: 2,
		},
		{
			name: "missing completion marker",
			result: &UnitTestResult{Classes: []UnitTestClass{{
				TestMethods: []UnitTestMethod{{ExecutionTime: 0.25}},
			}}},
		},
		{name: "nil unit result"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := analyzeExecuteABAPUnitResult(tt.result)
			if got.Success != tt.wantOK {
				t.Fatalf("Success = %t, want %t; message=%q", got.Success, tt.wantOK, got.Message)
			}
			if len(got.Output) != len(tt.wantOutput) {
				t.Fatalf("Output = %v, want %v", got.Output, tt.wantOutput)
			}
			for i := range tt.wantOutput {
				if got.Output[i] != tt.wantOutput[i] {
					t.Fatalf("Output = %v, want %v", got.Output, tt.wantOutput)
				}
			}
			if len(got.Alerts) != tt.wantAlerts {
				t.Fatalf("alerts = %d, want %d", len(got.Alerts), tt.wantAlerts)
			}
			if !got.Success && got.Message == "" {
				t.Fatal("failure did not include a diagnostic message")
			}
		})
	}
}

func unitResultWithMethodAlerts(alerts ...UnitTestAlert) *UnitTestResult {
	return &UnitTestResult{Classes: []UnitTestClass{{
		TestMethods: []UnitTestMethod{{ExecutionTime: 0.125, Alerts: alerts}},
	}}}
}
