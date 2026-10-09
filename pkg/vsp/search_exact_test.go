package vsp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

func TestSearchObjectsExact(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?><adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core">` +
			`<adtcore:objectReference adtcore:uri="/sap/bc/adt/oo/classes/zcl_order" adtcore:type="CLAS/OC" adtcore:name="ZCL_ORDER"/>` +
			`<adtcore:objectReference adtcore:uri="/sap/bc/adt/oo/classes/zcl_order_x" adtcore:type="CLAS/OC" adtcore:name="ZCL_ORDER_X"/>` +
			`</adtcore:objectReferences>`))
	}))
	defer srv.Close()
	client := adt.NewClient(srv.URL, "u", "p")

	got, err := searchObjects(context.Background(), client, "zcl_order", "", 100, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "ZCL_ORDER" {
		t.Fatalf("--exact: got %+v", got)
	}
	got, _ = searchObjects(context.Background(), client, "zcl_order", "", 100, false)
	if len(got) != 2 {
		t.Fatalf("without --exact the pattern search is unchanged, got %+v", got)
	}
	if searchCmd.Flags().Lookup("exact") == nil {
		t.Fatal("search has no --exact flag")
	}
}
