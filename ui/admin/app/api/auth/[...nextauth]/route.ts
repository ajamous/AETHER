// Auth.js v5 mounts its handlers on a single catch-all route.
// This file is the only piece that has to live on the filesystem;
// the actual logic lives in /auth.ts.

import { handlers } from '@/auth';

export const { GET, POST } = handlers;
