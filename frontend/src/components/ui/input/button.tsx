import React from "react";

type ButtonProps = {
  title: string;
  className?: string;
  handleClick?: () => void;
};

export const Button = ({ title, className, handleClick }: ButtonProps) => {
  return (
    <button
      className="hover:bg-blue-500 hover:text-white m-2 p-2 rounded-md transition-all"
      onClick={handleClick}
    >
      {title}
    </button>
  );
};
