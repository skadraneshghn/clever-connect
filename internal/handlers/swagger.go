package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ServeSwagger serves the interactive Swagger UI API documentation page
func ServeSwagger(c *gin.Context) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, swaggerHTML)
}

const swaggerHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>CleverConnect VPN Orchestrator - API Swagger Documentation</title>
    <link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui.css" />
    <link rel="icon" type="image/png" href="https://unpkg.com/swagger-ui-dist@5.11.0/favicon-32x32.png" sizes="32x32" />
    <link rel="icon" type="image/png" href="https://unpkg.com/swagger-ui-dist@5.11.0/favicon-16x16.png" sizes="16x16" />
    <style>
        html { box-sizing: border-box; overflow: -y-scroll; }
        *, *:before, *:after { box-sizing: inherit; }
        body { margin: 0; background: #fafafa; }
        .swagger-ui .topbar { display: none; } /* Hide default topbar */
        .header-custom {
            background: linear-gradient(135deg, #4f46e5 0%, #3b82f6 100%);
            color: #fff;
            padding: 20px 40px;
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
            display: flex;
            align-items: center;
            justify-content: space-between;
            box-shadow: 0 4px 15px rgba(0,0,0,0.1);
        }
        .header-custom h1 { margin: 0; font-size: 24px; font-weight: 700; letter-spacing: 0.5px; }
        .header-custom span { font-size: 13px; opacity: 0.85; }
    </style>
</head>
<body>
    <div class="header-custom">
        <div>
            <h1>CleverConnect Core API</h1>
            <span>Interactive Endpoint Console & Sandbox</span>
        </div>
        <div>
            <span>v1.0.0 (Stable)</span>
        </div>
    </div>
    
    <div id="swagger-ui"></div>

    <script src="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui-bundle.js" charset="UTF-8"></script>
    <script src="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui-standalone-preset.js" charset="UTF-8"></script>
    <script>
    window.onload = function() {
        // Embed OpenAPI Spec directly as JSON object
        const spec = {
            "openapi": "3.0.3",
            "info": {
                "title": "CleverConnect VPN Orchestrator APIs",
                "description": "Comprehensive specification for managing secure high-speed Ehco tunneling multiplexers, interactive SOCKS5 gateways, real-time log channels, and enterprise-grade sandboxed files manager.",
                "version": "1.0.0"
            },
            "servers": [
                {
                    "url": "/",
                    "description": "Relative server host"
                }
            ],
            "paths": {
                "/api/auth/login": {
                    "post": {
                        "summary": "Authenticate User",
                        "description": "Validates administrator credentials and returns a secure JWT payload.",
                        "requestBody": {
                            "required": true,
                            "content": {
                                "application/json": {
                                    "schema": {
                                        "type": "object",
                                        "required": ["username", "password"],
                                        "properties": {
                                            "username": { "type": "string", "example": "salman" },
                                            "password": { "type": "string", "example": "136517" }
                                        }
                                    }
                                }
                            }
                        },
                        "responses": {
                            "200": {
                                "description": "Successfully authenticated",
                                                        "content": {
                                    "application/json": {
                                        "schema": {
                                            "type": "object",
                                            "properties": {
                                                "token": { "type": "string", "example": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..." }
                                            }
                                        }
                                    }
                                }
                            },
                            "401": { "description": "Invalid credentials" }
                        }
                    }
                },
                "/api/ehco/config": {
                    "get": {
                        "summary": "Get Tunnel Configuration",
                        "description": "Retrieves the active Ehco tunneling engine configurations and daemon process status.",
                        "security": [{ "BearerAuth": [] }],
                        "responses": {
                            "200": {
                                "description": "Successfully loaded configuration",
                                "content": {
                                    "application/json": {
                                        "schema": {
                                            "type": "object",
                                            "properties": {
                                                "is_running": { "type": "boolean", "example": true },
                                                "config": {
                                                    "type": "object",
                                                    "properties": {
                                                        "local_port": { "type": "string", "example": "1080" },
                                                        "remote_url": { "type": "string", "example": "wss://cc.app.io/tunnel" },
                                                        "auth_token": { "type": "string" },
                                                        "enable_mux": { "type": "boolean" },
                                                        "keep_alive": { "type": "integer" }
                                                    }
                                                }
                                            }
                                        }
                                    }
                                }
                            }
                        }
                    },
                    "post": {
                        "summary": "Update and Restart Tunnel Config",
                        "description": "Saves new tunnel settings and dynamically restarts the Ehco proxy engine daemon.",
                        "security": [{ "BearerAuth": [] }],
                        "requestBody": {
                            "required": true,
                            "content": {
                                "application/json": {
                                    "schema": {
                                        "type": "object",
                                        "properties": {
                                            "local_port": { "type": "string", "example": "1080" },
                                            "remote_url": { "type": "string", "example": "wss://cc.app.io/tunnel" },
                                            "auth_token": { "type": "string", "example": "secret-token" },
                                            "enable_mux": { "type": "boolean", "example": true },
                                            "keep_alive": { "type": "integer", "example": 15 },
                                            "bypass_ir": { "type": "boolean", "example": true }
                                        }
                                    }
                                }
                            }
                        },
                        "responses": {
                            "200": { "description": "Configurations saved and daemon reloaded" }
                        }
                    }
                },
                "/api/files/list": {
                    "get": {
                        "summary": "List Sandbox Directory",
                        "description": "Lists directory nodes, files, properties, and overall server HDD disk space status.",
                        "security": [{ "BearerAuth": [] }],
                        "parameters": [
                            {
                                "name": "path",
                                "in": "query",
                                "description": "Subdirectory offset within the sandbox space",
                                "required": false,
                                "schema": { "type": "string", "example": "/documents" }
                            }
                        ],
                        "responses": {
                            "200": { "description": "Directory listing and disk stats returned" }
                        }
                    }
                },
                "/api/files/stream": {
                    "get": {
                        "summary": "Stream or Download File",
                        "description": "Streams binary files with support for HTTP Range multi-seek connections.",
                        "security": [{ "BearerAuth": [] }],
                        "parameters": [
                            {
                                "name": "path",
                                "in": "query",
                                "required": true,
                                "schema": { "type": "string" }
                            },
                            {
                                "name": "download",
                                "in": "query",
                                "description": "Set to true to force browser attachment download",
                                "schema": { "type": "string", "example": "true" }
                            }
                        ],
                        "responses": {
                            "200": { "description": "Binary payload stream" }
                        }
                    }
                },
                "/api/files/content": {
                    "get": {
                        "summary": "Read Text File Content",
                        "security": [{ "BearerAuth": [] }],
                        "parameters": [
                            { "name": "path", "in": "query", "required": true, "schema": { "type": "string" } }
                        ],
                        "responses": {
                            "200": { "description": "Raw text output payload" }
                        }
                    }
                },
                "/api/files/save": {
                    "post": {
                        "summary": "Save File Changes",
                        "security": [{ "BearerAuth": [] }],
                        "requestBody": {
                            "required": true,
                            "content": {
                                "application/json": {
                                    "schema": {
                                        "type": "object",
                                        "required": ["path", "content"],
                                        "properties": {
                                            "path": { "type": "string" },
                                            "content": { "type": "string" }
                                        }
                                    }
                                }
                            }
                        },
                        "responses": {
                            "200": { "description": "Saved" }
                        }
                    }
                },
                "/api/files/create-folder": {
                    "post": {
                        "summary": "Create Folder Node",
                        "security": [{ "BearerAuth": [] }],
                        "requestBody": {
                            "required": true,
                            "content": {
                                "application/json": {
                                    "schema": {
                                        "type": "object",
                                        "required": ["folder_name"],
                                        "properties": {
                                            "parent_path": { "type": "string" },
                                            "folder_name": { "type": "string" }
                                        }
                                    }
                                }
                            }
                        },
                        "responses": {
                            "200": { "description": "Folder created" }
                        }
                    }
                },
                "/api/files/compress": {
                    "post": {
                        "summary": "Compress to ZIP Archive",
                        "security": [{ "BearerAuth": [] }],
                        "requestBody": {
                            "required": true,
                            "content": {
                                "application/json": {
                                    "schema": {
                                        "type": "object",
                                        "required": ["items", "zip_name"],
                                        "properties": {
                                            "parent_path": { "type": "string" },
                                            "items": { "type": "array", "items": { "type": "string" } },
                                            "zip_name": { "type": "string", "example": "archive.zip" }
                                        }
                                    }
                                }
                            }
                        },
                        "responses": {
                            "200": { "description": "ZIP file compiled" }
                        }
                    }
                },
                "/api/files/decompress": {
                    "post": {
                        "summary": "Extract ZIP Archive",
                        "security": [{ "BearerAuth": [] }],
                        "requestBody": {
                            "required": true,
                            "content": {
                                "application/json": {
                                    "schema": {
                                        "type": "object",
                                        "required": ["path"],
                                        "properties": {
                                            "path": { "type": "string" }
                                        }
                                    }
                                }
                            }
                        },
                        "responses": {
                            "200": { "description": "ZIP unpacked" }
                        }
                    }
                }
            },
            "components": {
                "securitySchemes": {
                    "BearerAuth": {
                        "type": "http",
                        "scheme": "bearer",
                        "bearerFormat": "JWT"
                    }
                }
            }
        };

        const ui = SwaggerUIBundle({
            spec: spec,
            dom_id: '#swagger-ui',
            deepLinking: true,
            presets: [
                SwaggerUIBundle.presets.apis,
                SwaggerUIStandalonePreset
            ],
            plugins: [
                SwaggerUIBundle.plugins.DownloadUrl
            ],
            layout: "BaseLayout"
        });
        window.ui = ui;
    };
    </script>
</body>
</html>`
