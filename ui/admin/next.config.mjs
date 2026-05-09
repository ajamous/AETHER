/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  poweredByHeader: false,
  output: 'standalone',
  // Lab default; production sets AETHER_GATEWAY_URL.
  env: {
    AETHER_GATEWAY_URL: process.env.AETHER_GATEWAY_URL || 'http://localhost:8080',
    AETHER_AUDIT_URL: process.env.AETHER_AUDIT_URL || 'http://localhost:8447',
    AETHER_CERTMGR_URL: process.env.AETHER_CERTMGR_URL || 'http://localhost:8444',
  },
};

export default nextConfig;
