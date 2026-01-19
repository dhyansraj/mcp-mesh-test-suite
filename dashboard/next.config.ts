import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // Proxy API requests to tsuite backend
  async rewrites() {
    return [
      {
        source: "/api/:path*",
        destination: `${process.env.NEXT_PUBLIC_API_URL || "http://localhost:9999"}/api/:path*`,
      },
    ];
  },
};

export default nextConfig;
