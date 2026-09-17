package adt

import "testing"

// TestBuildObjectURLWithParent_NamespaceCasing covers the fix carried over
// during the v2.58.0 reconciliation: SAP addresses customer namespace
// objects (/NAMESPACE/...) in uppercase, and lowercasing them (the old,
// unconditional behavior) produces a URL SAP does not recognize. Ordinary
// Z objects must keep the pre-existing lowercase behavior.
func TestBuildObjectURLWithParent_NamespaceCasing(t *testing.T) {
	c := &Client{}

	tests := []struct {
		name       string
		objType    CreatableObjectType
		objName    string
		parentName string
		want       string
		wantErr    bool
	}{
		{
			name:    "namespaced class stays uppercase",
			objType: ObjectTypeClass,
			objName: "/NAMESPACE/CL_DEMO_CORE",
			want:    "/sap/bc/adt/oo/classes/%2FNAMESPACE%2FCL_DEMO_CORE",
		},
		{
			name:    "namespaced class given lowercase is still uppercased",
			objType: ObjectTypeClass,
			objName: "/namespace/cl_demo_core",
			want:    "/sap/bc/adt/oo/classes/%2FNAMESPACE%2FCL_DEMO_CORE",
		},
		{
			name:    "plain Z class is lowercased as before",
			objType: ObjectTypeClass,
			objName: "ZCL_DEMO_CORE",
			want:    "/sap/bc/adt/oo/classes/zcl_demo_core",
		},
		{
			name:       "namespaced function module: both name and parent group stay uppercase",
			objType:    ObjectTypeFunctionMod,
			objName:    "/NAMESPACE/FM_DEMO",
			parentName: "/NAMESPACE/FG_DEMO",
			want:       "/sap/bc/adt/functions/groups/%2FNAMESPACE%2FFG_DEMO/fmodules/%2FNAMESPACE%2FFM_DEMO",
		},
		{
			name:       "plain Z function module keeps lowercase group and module",
			objType:    ObjectTypeFunctionMod,
			objName:    "Z_FM_DEMO",
			parentName: "ZFG_DEMO",
			want:       "/sap/bc/adt/functions/groups/zfg_demo/fmodules/z_fm_demo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := c.buildObjectURLWithParent(tt.objType, tt.objName, tt.parentName)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got URL %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("buildObjectURLWithParent(%q, %q) = %q, want %q", tt.objName, tt.parentName, got, tt.want)
			}
		})
	}
}
