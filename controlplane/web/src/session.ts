import { createContext, useContext } from "react";

// Real login identity for the current session -- resolved once from
// /api/v1/auth/me (backed by the real session cookie set at login), not a
// hardcoded "Admin" label. role gates the one write action a browser user
// has (setting the budget); viewer sees everything, can't change it.
export interface Session {
  username: string;
  role: "admin" | "viewer" | "";
}

export const SessionContext = createContext<Session>({ username: "", role: "" });

export function useSession(): Session {
  return useContext(SessionContext);
}
