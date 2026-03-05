package com.sap.mcp.proxy.config;

/**
 * Configuration holder for SAP connection settings.
 * Supports both command-line arguments and environment variables.
 */
public class ConnectionConfig {
    // Load balancing via message server
    private String msHost;
    private String msServ;
    private String r3Name;
    private String group;

    // Direct application server connection
    private String asHost;
    private String sysnr;

    // Common settings
    private String client;
    private String username;
    private String password;
    private String language;

    public ConnectionConfig() {
        this.language = "EN";
    }

    /**
     * Check if this is a direct application server connection (vs load balancing).
     */
    public boolean isDirectConnection() {
        return asHost != null && !asHost.isEmpty();
    }

    /**
     * Create configuration from command-line arguments.
     * Expected format: --key value
     */
    public static ConnectionConfig fromArgs(String[] args) {
        ConnectionConfig config = new ConnectionConfig();

        for (int i = 0; i < args.length; i++) {
            String arg = args[i];

            switch (arg) {
                // Load balancing settings
                case "--mshost":
                    if (i + 1 < args.length) {
                        config.setMsHost(args[++i]);
                    }
                    break;
                case "--msserv":
                    if (i + 1 < args.length) {
                        config.setMsServ(args[++i]);
                    }
                    break;
                case "--r3name":
                    if (i + 1 < args.length) {
                        config.setR3Name(args[++i]);
                    }
                    break;
                case "--group":
                    if (i + 1 < args.length) {
                        config.setGroup(args[++i]);
                    }
                    break;
                // Direct app server settings
                case "--ashost":
                    if (i + 1 < args.length) {
                        config.setAsHost(args[++i]);
                    }
                    break;
                case "--sysnr":
                    if (i + 1 < args.length) {
                        config.setSysnr(args[++i]);
                    }
                    break;
                // Common settings
                case "--client":
                    if (i + 1 < args.length) {
                        config.setClient(args[++i]);
                    }
                    break;
                case "--user":
                    if (i + 1 < args.length) {
                        config.setUsername(args[++i]);
                    }
                    break;
                case "--password":
                    if (i + 1 < args.length) {
                        config.setPassword(args[++i]);
                    }
                    break;
                case "--lang":
                    if (i + 1 < args.length) {
                        config.setLanguage(args[++i]);
                    }
                    break;
            }
        }

        return config;
    }

    /**
     * Create configuration from environment variables.
     */
    public static ConnectionConfig fromEnvironment() {
        ConnectionConfig config = new ConnectionConfig();

        // Load balancing settings
        config.setMsHost(getEnv("SAP_MSHOST"));
        config.setMsServ(getEnv("SAP_MSSERV"));
        config.setR3Name(getEnv("SAP_R3NAME"));
        config.setGroup(getEnv("SAP_GROUP"));

        // Direct app server settings
        config.setAsHost(getEnv("SAP_ASHOST"));
        config.setSysnr(getEnv("SAP_SYSNR", "00"));

        // Common settings
        config.setClient(getEnv("SAP_CLIENT"));
        config.setUsername(getEnv("SAP_USERNAME"));
        config.setPassword(getEnv("SAP_PASSWORD"));
        config.setLanguage(getEnv("SAP_LANGUAGE", "EN"));

        return config;
    }

    /**
     * Merge command-line args over environment config (args take precedence).
     */
    public static ConnectionConfig merge(ConnectionConfig envConfig, ConnectionConfig argsConfig) {
        ConnectionConfig merged = new ConnectionConfig();

        // Load balancing settings
        merged.setMsHost(coalesce(argsConfig.getMsHost(), envConfig.getMsHost()));
        merged.setMsServ(coalesce(argsConfig.getMsServ(), envConfig.getMsServ()));
        merged.setR3Name(coalesce(argsConfig.getR3Name(), envConfig.getR3Name()));
        merged.setGroup(coalesce(argsConfig.getGroup(), envConfig.getGroup()));

        // Direct app server settings
        merged.setAsHost(coalesce(argsConfig.getAsHost(), envConfig.getAsHost()));
        merged.setSysnr(coalesce(argsConfig.getSysnr(), envConfig.getSysnr()));

        // Common settings
        merged.setClient(coalesce(argsConfig.getClient(), envConfig.getClient()));
        merged.setUsername(coalesce(argsConfig.getUsername(), envConfig.getUsername()));
        merged.setPassword(coalesce(argsConfig.getPassword(), envConfig.getPassword()));
        merged.setLanguage(coalesce(argsConfig.getLanguage(), envConfig.getLanguage()));

        return merged;
    }

    public void validate() {
        StringBuilder errors = new StringBuilder();

        // Check for either direct connection or load balancing settings
        boolean hasDirect = !isEmpty(asHost);
        boolean hasLoadBalancing = !isEmpty(msHost);

        if (!hasDirect && !hasLoadBalancing) {
            errors.append("Either asHost (direct) or msHost (load balancing) is required\n");
        }

        if (hasDirect && hasLoadBalancing) {
            errors.append("Cannot specify both asHost (direct) and msHost (load balancing) - choose one mode\n");
        }

        if (hasDirect) {
            // Direct connection validation
            if (isEmpty(sysnr)) errors.append("sysnr is required for direct connection\n");
        } else if (hasLoadBalancing) {
            // Load balancing validation
            if (isEmpty(msServ)) errors.append("msServ is required for load balancing\n");
            if (isEmpty(r3Name)) errors.append("r3Name is required for load balancing\n");
            if (isEmpty(group)) errors.append("group is required for load balancing\n");
        }

        // Common validation
        if (isEmpty(client)) errors.append("client is required\n");
        if (isEmpty(username)) errors.append("username is required\n");
        if (isEmpty(password)) errors.append("password is required\n");

        if (errors.length() > 0) {
            throw new IllegalArgumentException("Invalid configuration:\n" + errors);
        }
    }

    private static String getEnv(String key) {
        return System.getenv(key);
    }

    private static String getEnv(String key, String defaultValue) {
        String value = System.getenv(key);
        return value != null ? value : defaultValue;
    }

    private static String coalesce(String... values) {
        for (String v : values) {
            if (v != null && !v.isEmpty()) return v;
        }
        return null;
    }

    private static boolean isEmpty(String s) {
        return s == null || s.isEmpty();
    }

    // Getters and Setters - Load balancing
    public String getMsHost() { return msHost; }
    public void setMsHost(String msHost) { this.msHost = msHost; }

    public String getMsServ() { return msServ; }
    public void setMsServ(String msServ) { this.msServ = msServ; }

    public String getR3Name() { return r3Name; }
    public void setR3Name(String r3Name) { this.r3Name = r3Name; }

    public String getGroup() { return group; }
    public void setGroup(String group) { this.group = group; }

    // Getters and Setters - Direct connection
    public String getAsHost() { return asHost; }
    public void setAsHost(String asHost) { this.asHost = asHost; }

    public String getSysnr() { return sysnr; }
    public void setSysnr(String sysnr) { this.sysnr = sysnr; }

    // Getters and Setters - Common
    public String getClient() { return client; }
    public void setClient(String client) { this.client = client; }

    public String getUsername() { return username; }
    public void setUsername(String username) { this.username = username; }

    public String getPassword() { return password; }
    public void setPassword(String password) { this.password = password; }

    public String getLanguage() { return language; }
    public void setLanguage(String language) { this.language = language; }

    @Override
    public String toString() {
        StringBuilder sb = new StringBuilder("ConnectionConfig{");
        if (isDirectConnection()) {
            sb.append("mode='direct'")
              .append(", asHost='").append(asHost).append('\'')
              .append(", sysnr='").append(sysnr).append('\'');
        } else {
            sb.append("mode='loadBalancing'")
              .append(", msHost='").append(msHost).append('\'')
              .append(", msServ='").append(msServ).append('\'')
              .append(", r3Name='").append(r3Name).append('\'')
              .append(", group='").append(group).append('\'');
        }
        sb.append(", client='").append(client).append('\'')
          .append(", username='").append(username).append('\'')
          .append(", language='").append(language).append('\'')
          .append('}');
        return sb.toString();
    }
}
