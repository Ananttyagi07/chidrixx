import { supabase } from "./supabaseClient";

// Every authenticated API call goes through this instead of raw fetch --
// it attaches the real current Supabase session's access token as a
// Bearer header, which is what the control plane's requireSession now
// checks first (controlplane/auth.go). supabase-js keeps the session
// (and its automatic refresh) in localStorage on its own; this just
// reads whatever's current at call time, never a stale cached token.
export async function apiFetch(path: string, init: RequestInit = {}): Promise<Response> {
  const {
    data: { session },
  } = await supabase.auth.getSession();

  const headers = new Headers(init.headers);
  if (session?.access_token) {
    headers.set("Authorization", `Bearer ${session.access_token}`);
  }

  return fetch(path, { ...init, headers });
}
