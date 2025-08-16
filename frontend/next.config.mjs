/** @type {import('next').NextConfig} */
const nextConfig = {
  output: "standalone",
  images: {
    remotePatterns: [
      {
        protocol: "https",
        hostname: "j5f7.c18.e2-3.dev",
        pathname: "/shares/**",
      },
      {
        protocol: "https",
        hostname: "shares.j5f7.c18.e2-3.dev",
        pathname: "/**", // Permitir todas las rutas del dominio
      },
      {
        protocol: "https",
        hostname: "m1l6.fra5.idrivee2-8.com",
        pathname: "/**", // Permitir todas las rutas del dominio
      },
      {
        protocol: "https",
        hostname: "shares.m1l6.fra5.idrivee2-8.com",
        pathname: "/**", // Permitir todas las rutas del dominio
      },
      {
        protocol: "https",
        hostname: "tokenfleet.io",
        pathname: "/**", // Permitir todas las rutas del dominio
      },
      {
        protocol: "https",
        hostname: "app.tokenfleet.io",
        pathname: "/**", // Permitir todas las rutas del dominio
      },
      {
        protocol: "https",
        hostname: "production.j5f7.c18.e2-3.dev",
        pathname: "/**",
      },
      {
        protocol: "https",
        hostname: "j5f7.c18.e2-3.dev",
        pathname: "/**",
      },
      {
        protocol: "https",
        hostname: "metadata.tokenfleet.io",
        pathname: "/**",
      },
      {
        protocol: "https",
        hostname: "metadata.tokenfleet.xyz",
        pathname: "/**",
      },
    ],
  },
  webpack: (config) => {
    config.externals.push("pino-pretty", "lokijs", "encoding");
    return config;
  },
};

export default nextConfig;
