package com.sap.mcp.proxy;

import com.sap.conn.jco.*;
import com.sap.mcp.proxy.model.RfcCallRequest;
import com.sap.mcp.proxy.model.RfcCallResponse;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.*;

/**
 * Calls arbitrary SAP function modules directly via JCo.
 * Used for: CallRFC, RunReport, GetVariants, SetTextElements.
 *
 * Unlike RestRfcEndpointCaller (which calls SADT_REST_RFC_ENDPOINT to proxy HTTP),
 * this caller invokes function modules directly, giving access to all SAP RFC-enabled FMs.
 */
public class DirectRfcCaller {
    private static final Logger logger = LoggerFactory.getLogger(DirectRfcCaller.class);

    private final JCoConnectionManager connectionManager;

    public DirectRfcCaller(JCoConnectionManager connectionManager) {
        this.connectionManager = connectionManager;
    }

    /**
     * Execute a function module call.
     *
     * @param request The RFC call request with function name and parameters
     * @return Response with export parameters and tables
     */
    public RfcCallResponse execute(RfcCallRequest request) {
        if (request.getFunction() == null || request.getFunction().isEmpty()) {
            return RfcCallResponse.error("function name is required");
        }

        logger.info("Executing RFC call: {}", request.getFunction());
        long startTime = System.currentTimeMillis();

        try {
            JCoDestination destination = connectionManager.getDestination();
            JCoFunction function = destination.getRepository().getFunction(request.getFunction());

            if (function == null) {
                return RfcCallResponse.error("Function module '" + request.getFunction() +
                        "' not found. Check if it exists and is RFC-enabled.");
            }

            // Set import parameters
            if (request.getParams() != null) {
                populateImportParams(function, request.getParams());
            }

            // Execute
            function.execute(destination);
            long duration = System.currentTimeMillis() - startTime;
            logger.info("RFC call {} completed in {}ms", request.getFunction(), duration);

            // Extract results
            Map<String, Object> exports = extractExportParams(function);
            Map<String, List<Map<String, Object>>> tables = extractTables(function);

            return RfcCallResponse.success(exports, tables, duration);

        } catch (JCoException e) {
            logger.error("RFC call failed: {}", e.getMessage(), e);
            return RfcCallResponse.error("RFC call failed: " + e.getMessage());
        } catch (Exception e) {
            logger.error("Unexpected error: {}", e.getMessage(), e);
            return RfcCallResponse.error("Unexpected error: " + e.getMessage());
        }
    }

    /**
     * Populate function import parameters from the request params map.
     * Handles scalar values and table parameters (arrays of objects).
     */
    @SuppressWarnings("unchecked")
    private void populateImportParams(JCoFunction function, Map<String, Object> params) {
        JCoParameterList importParams = function.getImportParameterList();
        JCoParameterList tableParams = function.getTableParameterList();

        for (Map.Entry<String, Object> entry : params.entrySet()) {
            String name = entry.getKey();
            Object value = entry.getValue();

            try {
                // Check if it's a table parameter (array of objects)
                if (value instanceof List) {
                    if (tableParams != null) {
                        JCoTable table = tableParams.getTable(name);
                        List<Map<String, Object>> rows = (List<Map<String, Object>>) value;
                        for (Map<String, Object> row : rows) {
                            table.appendRow();
                            for (Map.Entry<String, Object> col : row.entrySet()) {
                                table.setValue(col.getKey(), String.valueOf(col.getValue()));
                            }
                        }
                        logger.debug("Set table parameter {} with {} rows", name, rows.size());
                    }
                } else if (value instanceof Map) {
                    // Structure parameter
                    if (importParams != null) {
                        JCoStructure struct = importParams.getStructure(name);
                        Map<String, Object> fields = (Map<String, Object>) value;
                        for (Map.Entry<String, Object> field : fields.entrySet()) {
                            struct.setValue(field.getKey(), String.valueOf(field.getValue()));
                        }
                        logger.debug("Set structure parameter {}", name);
                    }
                } else {
                    // Scalar parameter
                    if (importParams != null) {
                        importParams.setValue(name, String.valueOf(value));
                        logger.debug("Set import parameter {} = {}", name, value);
                    }
                }
            } catch (Exception e) {
                logger.warn("Failed to set parameter {}: {}", name, e.getMessage());
            }
        }
    }

    /**
     * Extract export parameters from the function result.
     */
    private Map<String, Object> extractExportParams(JCoFunction function) {
        Map<String, Object> exports = new LinkedHashMap<>();
        JCoParameterList exportParams = function.getExportParameterList();

        if (exportParams == null) {
            return exports;
        }

        for (int i = 0; i < exportParams.getFieldCount(); i++) {
            JCoField field = exportParams.getField(i);
            try {
                if (field.isStructure()) {
                    exports.put(field.getName(), structureToMap(field.getStructure()));
                } else if (field.isTable()) {
                    exports.put(field.getName(), tableToList(field.getTable()));
                } else {
                    exports.put(field.getName(), field.getString());
                }
            } catch (Exception e) {
                logger.warn("Error reading export param {}: {}", field.getName(), e.getMessage());
                exports.put(field.getName(), field.getString());
            }
        }

        return exports;
    }

    /**
     * Extract table parameters from the function result.
     */
    private Map<String, List<Map<String, Object>>> extractTables(JCoFunction function) {
        Map<String, List<Map<String, Object>>> tables = new LinkedHashMap<>();
        JCoParameterList tableParams = function.getTableParameterList();

        if (tableParams == null) {
            return tables;
        }

        for (int i = 0; i < tableParams.getFieldCount(); i++) {
            JCoField field = tableParams.getField(i);
            try {
                if (field.isTable()) {
                    tables.put(field.getName(), tableToList(field.getTable()));
                }
            } catch (Exception e) {
                logger.warn("Error reading table {}: {}", field.getName(), e.getMessage());
            }
        }

        return tables;
    }

    /**
     * Convert a JCo structure to a map.
     */
    private Map<String, Object> structureToMap(JCoStructure structure) {
        Map<String, Object> map = new LinkedHashMap<>();
        for (int i = 0; i < structure.getFieldCount(); i++) {
            map.put(structure.getField(i).getName(), structure.getString(i));
        }
        return map;
    }

    /**
     * Convert a JCo table to a list of maps.
     */
    private List<Map<String, Object>> tableToList(JCoTable table) {
        List<Map<String, Object>> rows = new ArrayList<>();
        for (int i = 0; i < table.getNumRows(); i++) {
            table.setRow(i);
            Map<String, Object> row = new LinkedHashMap<>();
            for (int j = 0; j < table.getFieldCount(); j++) {
                row.put(table.getField(j).getName(), table.getString(j));
            }
            rows.add(row);
        }
        return rows;
    }
}
