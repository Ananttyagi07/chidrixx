import { createClient } from "@supabase/supabase-js";

// Real Supabase project, real publishable key -- safe to embed in the
// built bundle (it's the public/anon-equivalent key; the secret key
// never goes anywhere near the frontend). Missing env vars fail loudly
// at startup rather than silently falling back to a fake client that
// would make every auth call fail mysteriously.
const url = import.meta.env.VITE_SUPABASE_URL;
const publishableKey = import.meta.env.VITE_SUPABASE_PUBLISHABLE_KEY;

if (!url || !publishableKey) {
  throw new Error(
    "VITE_SUPABASE_URL and VITE_SUPABASE_PUBLISHABLE_KEY must be set (controlplane/web/.env.local) — real Supabase auth has no fallback.",
  );
}

export const supabase = createClient(url, publishableKey);
