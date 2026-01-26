"use client";

interface LogoProps {
  className?: string;
  size?: number;
}

export function Logo({ className = "", size = 32 }: LogoProps) {
  return (
    <img
      src="/logo.svg"
      alt="tsuite logo"
      width={size}
      height={size}
      className={className}
    />
  );
}
