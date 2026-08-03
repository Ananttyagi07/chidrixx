// Shared constants between globalSetup and the test files -- fixed
// rather than randomly generated, since this whole suite runs against
// one real, disposable control plane instance per test run, not a
// shared environment where collisions would matter.
import path from "node:path";
import os from "node:os";

export const PORT = 19123;
export const BASE_URL = `http://localhost:${PORT}`;

export const E2E_DIR = path.join(os.tmpdir(), "chidrixx-e2e");
export const DB_PATH = path.join(E2E_DIR, "e2e.db");
export const BINARY_PATH = path.join(E2E_DIR, "controlplane-e2e");
export const PID_FILE = path.join(E2E_DIR, "server.pid");
export const INGEST_TOKEN_FILE = path.join(E2E_DIR, "ingest-token.txt");

export const ADMIN_USER = "e2e-admin";
export const ADMIN_PASSWORD = "e2e-admin-password-123";
export const VIEWER_USER = "e2e-viewer";
export const VIEWER_PASSWORD = "e2e-viewer-password-123";
export const TENANT_NAME = "e2e-tenant";
