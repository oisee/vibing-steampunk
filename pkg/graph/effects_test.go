package graph

import "testing"

func TestExtractEffects_Pure(t *testing.T) {
	source := `METHOD validate_dates.
  IF iv_end < iv_start.
    rv_valid = abap_false.
  ELSE.
    rv_valid = abap_true.
  ENDIF.
ENDMETHOD.`
	e := ExtractEffects(source)
	if !e.IsPure() {
		t.Fatal("validate_dates should be pure")
	}
	if e.LUWClass != "safe" {
		t.Fatalf("LUWClass = %q, want safe", e.LUWClass)
	}
}

func TestExtractEffects_ReadsDB(t *testing.T) {
	source := `METHOD get_order.
  SELECT SINGLE * FROM zorders INTO rs_order WHERE id = iv_id.
ENDMETHOD.`
	e := ExtractEffects(source)
	if e.IsPure() {
		t.Fatal("should not be pure — reads DB")
	}
	if len(e.ReadsDB) != 1 || e.ReadsDB[0] != "ZORDERS" {
		t.Fatalf("ReadsDB = %v, want [ZORDERS]", e.ReadsDB)
	}
	if e.LUWClass != "safe" {
		t.Fatalf("LUWClass = %q, want safe (reading is LUW-safe)", e.LUWClass)
	}
}

func TestExtractEffects_WritesDB(t *testing.T) {
	source := `METHOD save_order.
  INSERT zorders FROM ls_order.
  MODIFY zorder_items FROM TABLE lt_items.
ENDMETHOD.`
	e := ExtractEffects(source)
	if len(e.WritesDB) != 2 {
		t.Fatalf("WritesDB = %v, want 2 tables", e.WritesDB)
	}
}

func TestExtractEffects_CommitOwner(t *testing.T) {
	source := `METHOD process.
  INSERT zorders FROM ls_order.
  COMMIT WORK AND WAIT.
ENDMETHOD.`
	e := ExtractEffects(source)
	if !e.HasCommit {
		t.Fatal("should detect COMMIT WORK")
	}
	if e.LUWClass != "owner" {
		t.Fatalf("LUWClass = %q, want owner", e.LUWClass)
	}
}

func TestExtractEffects_UpdateTask(t *testing.T) {
	source := `METHOD save_deferred.
  CALL FUNCTION 'Z_SAVE_ORDER'
    IN UPDATE TASK
    EXPORTING is_order = ls_order.
ENDMETHOD.`
	e := ExtractEffects(source)
	if !e.UpdateTask {
		t.Fatal("should detect IN UPDATE TASK")
	}
	if e.LUWClass != "participant" {
		t.Fatalf("LUWClass = %q, want participant", e.LUWClass)
	}
}

func TestExtractEffects_LUWUnsafe(t *testing.T) {
	source := `METHOD bad_pattern.
  CALL FUNCTION 'Z_SAVE'
    IN UPDATE TASK
    EXPORTING iv = lv.
  COMMIT WORK.
ENDMETHOD.`
	e := ExtractEffects(source)
	if e.LUWClass != "unsafe" {
		t.Fatalf("LUWClass = %q, want unsafe (commit + update task)", e.LUWClass)
	}
}

func TestExtractEffects_AsyncRFC(t *testing.T) {
	source := `METHOD fire_and_forget.
  CALL FUNCTION 'Z_NOTIFY'
    STARTING NEW TASK 'TASK1'
    EXPORTING iv_msg = lv_msg.
ENDMETHOD.`
	e := ExtractEffects(source)
	if !e.AsyncRFC {
		t.Fatal("should detect STARTING NEW TASK")
	}
}

func TestExtractEffects_BackgroundJob(t *testing.T) {
	source := `METHOD schedule.
  SUBMIT zreport VIA JOB lv_job NUMBER lv_num AND RETURN.
ENDMETHOD.`
	e := ExtractEffects(source)
	if !e.BackgroundJob {
		t.Fatal("should detect VIA JOB")
	}
	if !e.SubmitAndReturn {
		t.Fatal("should detect AND RETURN")
	}
}

func TestExtractEffects_RFC(t *testing.T) {
	source := `METHOD call_remote.
  CALL FUNCTION 'BAPI_USER_GET_DETAIL'
    DESTINATION 'RFC_DEST'
    EXPORTING username = lv_user
    IMPORTING address = ls_addr.
ENDMETHOD.`
	e := ExtractEffects(source)
	if len(e.SyncRFC) != 1 || e.SyncRFC[0] != "RFC_DEST" {
		t.Fatalf("SyncRFC = %v, want [RFC_DEST]", e.SyncRFC)
	}
}

func TestExtractEffects_RaiseException(t *testing.T) {
	source := `METHOD validate.
  IF iv_value IS INITIAL.
    RAISE EXCEPTION TYPE zcx_validation
      EXPORTING textid = zcx_validation=>empty_value.
  ENDIF.
ENDMETHOD.`
	e := ExtractEffects(source)
	if !e.RaisesExc {
		t.Fatal("should detect RAISE EXCEPTION")
	}
}

func TestExtractEffects_LeaveTransaction(t *testing.T) {
	source := `METHOD navigate.
  LEAVE TO TRANSACTION 'VA03'.
ENDMETHOD.`
	e := ExtractEffects(source)
	if !e.LeavesContext {
		t.Fatal("should detect LEAVE TO TRANSACTION")
	}
}

func TestExtractEffects_Complex(t *testing.T) {
	source := `METHOD process_order.
  DATA lo_http TYPE REF TO if_http_client.
  SELECT SINGLE * FROM zorders INTO ls_order WHERE id = iv_id.
  ls_order-status = 'P'.
  UPDATE zorders FROM ls_order.
  CALL FUNCTION 'Z_AUDIT_LOG'
    IN UPDATE TASK
    EXPORTING iv_action = 'PROCESS' iv_id = iv_id.
  CALL FUNCTION 'Z_NOTIFY_ERP'
    DESTINATION 'ERP_PROD'
    EXPORTING is_order = ls_order.
  COMMIT WORK AND WAIT.
ENDMETHOD.`
	e := ExtractEffects(source)

	if len(e.ReadsDB) != 1 || e.ReadsDB[0] != "ZORDERS" {
		t.Fatalf("ReadsDB = %v, want [ZORDERS]", e.ReadsDB)
	}
	if len(e.WritesDB) != 1 || e.WritesDB[0] != "ZORDERS" {
		t.Fatalf("WritesDB = %v, want [ZORDERS]", e.WritesDB)
	}
	if !e.UpdateTask {
		t.Fatal("should detect IN UPDATE TASK")
	}
	if len(e.SyncRFC) != 1 || e.SyncRFC[0] != "ERP_PROD" {
		t.Fatalf("SyncRFC = %v, want [ERP_PROD]", e.SyncRFC)
	}
	if !e.HasCommit {
		t.Fatal("should detect COMMIT")
	}
	if e.LUWClass != "unsafe" {
		t.Fatalf("LUWClass = %q, want unsafe (commit + update task)", e.LUWClass)
	}
}

func TestExtractEffects_DeleteDB(t *testing.T) {
	source := `METHOD cleanup.
  DELETE FROM zorders WHERE status = 'X'.
ENDMETHOD.`
	e := ExtractEffects(source)
	if len(e.WritesDB) != 1 || e.WritesDB[0] != "ZORDERS" {
		t.Fatalf("WritesDB = %v, want [ZORDERS]", e.WritesDB)
	}
}

func TestExtractEffects_Rollback(t *testing.T) {
	source := `METHOD undo.
  ROLLBACK WORK.
ENDMETHOD.`
	e := ExtractEffects(source)
	if !e.HasRollback {
		t.Fatal("should detect ROLLBACK WORK")
	}
	if e.LUWClass != "owner" {
		t.Fatalf("LUWClass = %q, want owner", e.LUWClass)
	}
}
