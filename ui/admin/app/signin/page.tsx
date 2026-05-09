import { redirect } from 'next/navigation';
import { signIn, oidcEnabled } from '@/auth';

// Sign-in entry. Renders only the OIDC button when configured;
// when auth is disabled (lab mode), it bounces back to the
// dashboard immediately so the URL still works as a no-op.

export default function SignInPage() {
  if (!oidcEnabled) {
    redirect('/');
  }

  return (
    <div className="min-h-screen flex items-center justify-center px-4">
      <div className="w-full max-w-sm border border-zinc-200 dark:border-zinc-800 rounded-lg p-8">
        <div className="text-lg font-semibold tracking-tight">Aether</div>
        <div className="text-xs text-zinc-500 dark:text-zinc-400 mt-1 mb-6">
          Open Source RSP — admin
        </div>
        <p className="text-sm mb-6">
          Sign in with your operator identity provider.
        </p>
        <form
          action={async () => {
            'use server';
            await signIn('oidc', { redirectTo: '/' });
          }}
        >
          <button
            type="submit"
            className="w-full px-3 py-2 rounded bg-accent-600 text-white text-sm hover:bg-accent-700"
          >
            Sign in with OIDC
          </button>
        </form>
      </div>
    </div>
  );
}
