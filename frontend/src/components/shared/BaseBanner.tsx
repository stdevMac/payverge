"use client";
import { Card, CardBody } from "@nextui-org/react";
import { useEffect, useState } from "react";

interface Message {
  text: string;
  highlight: string;
}

interface Props {
  messages: Message[];
  sticky?: boolean;
  onClick?: (e: React.MouseEvent) => void;
}

export const BaseBanner = ({ messages, sticky = false, onClick }: Props) => {
  const [currentIndex, setCurrentIndex] = useState(0);
  const [prevIndex, setPrevIndex] = useState(0);
  const [isHovered, setIsHovered] = useState(false);

  useEffect(() => {
    const interval = setInterval(() => {
      setPrevIndex(currentIndex);
      setCurrentIndex((prevIndex) => (prevIndex + 1) % messages.length);
    }, 5000);

    return () => clearInterval(interval);
  }, [currentIndex, messages.length]);

  const renderMessage = (index: number, isLeaving: boolean) => {
    const messageToShow = messages[index % messages.length];
    return (
      <div
        key={`${index}-${isLeaving ? "leaving" : "entering"}-${messageToShow.text}`}
        className={`absolute w-full h-full flex flex-col sm:flex-row items-center justify-center gap-1 sm:gap-2 ${
          isLeaving ? "animate-slideToRight" : "animate-slideFromLeft"
        }`}
      >
        <span className="text-center sm:text-right text-white font-semibold text-sm sm:text-base whitespace-nowrap leading-none">
          {messageToShow.text}
        </span>
        <span className="text-white font-bold text-sm sm:text-base whitespace-nowrap leading-none">
          {messageToShow.highlight}
        </span>
      </div>
    );
  };

  return (
    <div className="w-full bg-gradient-to-r from-primary via-secondary to-primary relative">
      <div className="max-w-[1200px] mx-auto px-4">
        <Card
          className={`bg-transparent shadow-none cursor-pointer transition-all duration-300 border-none ${
            isHovered ? "scale-[1.01]" : ""
          } ${sticky ? "sticky top-0 z-50" : ""}`}
          onClick={onClick}
          onMouseEnter={() => setIsHovered(true)}
          onMouseLeave={() => setIsHovered(false)}
        >
          <div className="absolute inset-0 bg-[radial-gradient(circle,rgba(255,255,255,0.1)_1px,transparent_1px)] bg-[length:16px_16px]" />
          <div
            className={`absolute inset-0 ${isHovered ? "shine-effect" : ""}`}
          />
          <CardBody className="py-2 sm:py-3 relative overflow-hidden min-h-[56px] sm:min-h-[48px] flex items-center justify-center">
            <div className="relative w-full h-full">
              {renderMessage(prevIndex, true)}
              {renderMessage(currentIndex, false)}
            </div>
          </CardBody>
        </Card>
      </div>
    </div>
  );
};
