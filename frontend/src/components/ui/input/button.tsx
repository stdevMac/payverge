import React from "react";

type ButtonProps = {
  title: string;
  className?: string;
  handleClick?: () => void;
};

export const Button = ({
  title,
  className: _className,
  handleClick,
}: ButtonProps) => {
  return (
    <button
      className={`hover:bg-brand hover:text-white m-2 p-2 rounded-md transition-all ${_className || ""}`}
      onClick={handleClick}
    >
      {title}
    </button>
  );
};
