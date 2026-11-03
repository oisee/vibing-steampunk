package adt

import (
	"context"
	"strings"
	"testing"
)

func selectedTocClient(t *testing.T) (*Client, *tocServer, *fakeBridge) {
	t.Helper()
	source := strings.ReplaceAll(transportXML("TR-SRC", "demo", "D", map[string][]string{
		"TR-TASK1": {"/PAR/CL_IM_SCH_GI_POST_DPS", "/SCWM/CL_IM_LOAD_INSTR_PDF"},
	}), `tm:type="PROG"`, `tm:type="CLAS"`)
	client, ts := newTocClient(t, source, WithAllowedPackages("/PAR/*"))
	ts.objects = map[string][2]string{
		"/PAR/CL_IM_SCH_GI_POST_DPS": {"CLAS/OC", "/PAR/BC"},
		"/SCWM/CL_IM_LOAD_INSTR_PDF": {"CLAS/OC", "/SCWM/DELIVERY"},
	}
	ts.tocObjects = strings.ReplaceAll(transportXML("TR-TOC", "ToC", "D", map[string][]string{
		"TR-TARGET": {"/PAR/CL_IM_SCH_GI_POST_DPS"},
	}), `tm:type="PROG"`, `tm:type="CLAS"`)
	return client, ts, &fakeBridge{}
}

func TestCopyToTransportOfCopies_SelectedClassOnlyAndRelease(t *testing.T) {
	client, ts, bridge := selectedTocClient(t)
	res, err := client.copyToTransportOfCopies(context.Background(), bridge, "TR-SRC", TransportOfCopiesOptions{
		Target: "/PAR/", Release: true,
		Objects: []TransportObjectKey{{PgmID: "R3TR", Object: "CLAS", Name: "/PAR/CL_IM_SCH_GI_POST_DPS"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Released || len(ts.released) != 1 || len(res.Objects) != 1 || res.Objects[0].Name != "/PAR/CL_IM_SCH_GI_POST_DPS" {
		t.Fatalf("wrong selected ToC: %+v, releases %v", res, ts.released)
	}
	if len(res.CopiedEntries) != 1 || res.CopiedEntries[0] != "R3TR CLAS /PAR/CL_IM_SCH_GI_POST_DPS" {
		t.Fatalf("copied entries = %v", res.CopiedEntries)
	}
	if len(bridge.calls) != 1 || bridge.calls[0]["_fn"] != "TR_APPEND_TO_COMM_OBJS_KEYS" {
		t.Fatalf("selection used whole-list copying: %v", bridge.calls)
	}
	rows := bridge.calls[0]["WT_E071"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["OBJ_NAME"] != "/PAR/CL_IM_SCH_GI_POST_DPS" || bridge.calls[0]["WI_TRKORR"] != "TR-TOC" {
		t.Fatalf("wrong object list sent: %v", bridge.calls)
	}
}

func TestCopyToTransportOfCopies_SelectedTaskNormalizesAndDeduplicates(t *testing.T) {
	client, ts, bridge := selectedTocClient(t)
	ts.responses = map[string]string{
		"TR-TASK1": transportXML("TR-SRC", "demo", "D", nil),
	}
	res, err := client.copyToTransportOfCopies(context.Background(), bridge, "TR-TASK1", TransportOfCopiesOptions{
		Target: "/PAR/", Release: true,
		Objects: []TransportObjectKey{
			{"R3TR", "CLAS", "/PAR/CL_IM_SCH_GI_POST_DPS"},
			{"r3tr", "clas", "/par/cl_im_sch_gi_post_dps"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Released || len(res.Objects) != 1 || res.Objects[0].Name != "/PAR/CL_IM_SCH_GI_POST_DPS" ||
		len(res.CopiedFrom) != 1 || res.CopiedFrom[0] != "TR-TASK1" || len(res.CopiedEntries) != 1 {
		t.Fatalf("wrong selected task copy: %+v", res)
	}
	if len(bridge.calls) != 1 || len(bridge.calls[0]["WT_E071"].([]any)) != 1 {
		t.Fatalf("selection was not deduplicated: %v", bridge.calls)
	}
}

func TestCopyToTransportOfCopies_SelectionRefusesBeforeCreation(t *testing.T) {
	for _, tt := range []struct {
		name    string
		objects []TransportObjectKey
	}{
		{"whole forbidden request", nil},
		{"forbidden object", []TransportObjectKey{{"R3TR", "CLAS", "/SCWM/CL_IM_LOAD_INSTR_PDF"}}},
		{"absent object", []TransportObjectKey{{"R3TR", "CLAS", "/PAR/ABSENT"}}},
		{"empty selection", []TransportObjectKey{}},
		{"part", []TransportObjectKey{{"LIMU", "METH", "ZCL_TEST RUN"}}},
		{"table keys", []TransportObjectKey{{"R3TR", "TABU", "T000"}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client, ts, bridge := selectedTocClient(t)
			res, err := client.copyToTransportOfCopies(context.Background(), bridge, "TR-SRC", TransportOfCopiesOptions{
				Target: "/PAR/", Release: true, Objects: tt.objects,
			})
			if err == nil || res != nil || len(ts.created) != 0 || len(bridge.calls) != 0 || len(ts.released) != 0 {
				t.Fatalf("unsafe refusal: result %+v, error %v, creates %v, bridge %v, releases %v", res, err, ts.created, bridge.calls, ts.released)
			}
		})
	}
}

func TestCopyToTransportOfCopies_SelectionNeverReleasesUnverifiedContents(t *testing.T) {
	for _, mode := range []string{"extra", "missing", "wrong request", "unreadable", "append failed"} {
		t.Run(mode, func(t *testing.T) {
			client, ts, bridge := selectedTocClient(t)
			switch mode {
			case "extra":
				ts.tocObjects = transportXML("TR-TOC", "ToC", "D", map[string][]string{"TR-TARGET": {"ZUNEXPECTED"}})
			case "missing":
				ts.tocObjects = transportXML("TR-TOC", "ToC", "D", nil)
			case "wrong request":
				ts.tocObjects = strings.ReplaceAll(ts.tocObjects, `"TR-TOC"`, `"TR-OTHER"`)
			case "unreadable":
				ts.tocObjects = "<broken"
			case "append failed":
				bridge.subrc = 8
			}

			res, err := client.copyToTransportOfCopies(context.Background(), bridge, "TR-SRC", TransportOfCopiesOptions{
				Target: "/PAR/", Release: true,
				Objects: []TransportObjectKey{{"R3TR", "CLAS", "/PAR/CL_IM_SCH_GI_POST_DPS"}},
			})
			if err == nil || res == nil || res.Transport != "TR-TOC" || res.Released || len(ts.released) != 0 {
				t.Fatalf("unverified ToC released or lost: %+v, %v", res, err)
			}
		})
	}
}

func TestCopyToTransportOfCopies_TaskSelectionDoesNotBorrowFromSibling(t *testing.T) {
	client, ts, bridge := selectedTocClient(t)
	ts.responses = map[string]string{
		"TR-TASK1": transportXML("TR-SRC", "demo", "D", nil),
		"TR-SRC": strings.ReplaceAll(transportXML("TR-SRC", "demo", "D", map[string][]string{
			"TR-TASK1": {"/SCWM/CL_IM_LOAD_INSTR_PDF"},
			"TR-TASK2": {"/PAR/CL_IM_SCH_GI_POST_DPS"},
		}), `tm:type="PROG"`, `tm:type="CLAS"`),
	}
	_, err := client.copyToTransportOfCopies(context.Background(), bridge, "TR-TASK1", TransportOfCopiesOptions{
		Target: "/PAR/", Objects: []TransportObjectKey{{"R3TR", "CLAS", "/PAR/CL_IM_SCH_GI_POST_DPS"}},
	})
	if err == nil || !strings.Contains(err.Error(), "not in") || len(ts.created) != 0 || len(bridge.calls) != 0 {
		t.Fatalf("borrowed a sibling's class: error %v, creates %v, calls %v", err, ts.created, bridge.calls)
	}
}
