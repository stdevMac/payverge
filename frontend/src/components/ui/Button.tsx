"use client";

interface ButtonProps {
  children: React.ReactNode;
  variant?: 'primary' | 'secondary' | 'ghost';
  size?: 'default' | 'large';
  onClick?: () => void;
  className?: string;
  disabled?: boolean;
  type?: 'button' | 'submit';
}

export const Button = ({ 
  children, 
  variant = 'primary', 
  size = 'default',
  onClick, 
  className = '',
  disabled = false,
  type = 'button'
}: ButtonProps) => {
  const baseClasses = "font-medium transition-all duration-200 tracking-wide focus:outline-none focus:ring-2 focus:ring-offset-2";
  
  const sizeClasses = {
    default: "px-6 py-3 text-base",
    large: "px-8 py-4 text-lg"
  };
  
  const variantClasses = {
    primary: "bg-gray-900 text-white hover:bg-gray-800 focus:ring-gray-500 rounded-lg",
    secondary: "border border-gray-300 text-gray-700 hover:border-gray-400 hover:text-gray-900 bg-white rounded-lg",
    ghost: "text-gray-600 hover:text-gray-900 underline decoration-dotted underline-offset-4"
  };

  const disabledClasses = disabled ? "opacity-50 cursor-not-allowed" : "";

  return (
    <button
      type={type}
      onClick={onClick}
      disabled={disabled}
      className={`${baseClasses} ${sizeClasses[size]} ${variantClasses[variant]} ${disabledClasses} ${className}`}
    >
      {children}
    </button>
  );
};
