"use client";

interface BadgeProps {
  children: React.ReactNode;
  variant?: 'default' | 'primary';
  className?: string;
}

export const Badge = ({ children, variant = 'default', className = '' }: BadgeProps) => {
  const baseClasses = "inline-flex items-center gap-2 px-4 py-2 rounded-full text-sm font-medium";
  
  const variantClasses = {
    default: "bg-gray-50 border border-gray-200 text-gray-600",
    primary: "bg-blue-50 border border-blue-200 text-blue-600"
  };

  return (
    <div className={`${baseClasses} ${variantClasses[variant]} ${className}`}>
      <div className="w-2 h-2 bg-current rounded-full animate-pulse opacity-60"></div>
      {children}
    </div>
  );
};
