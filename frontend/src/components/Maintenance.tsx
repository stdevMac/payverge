"use client";

import Image from 'next/image';

export default function Maintenance() {
  return (
    <div className="min-h-screen flex flex-col justify-center items-center bg-gradient-to-b from-gray-50 to-blue-100">
      <div className="bg-white rounded-2xl shadow-2xl p-8 sm:p-12 mb-6 sm:mb-8 max-w-lg text-center hover:shadow-blue-200 transition-shadow duration-300">
        <Image
          src="/footer-icons/PayvergeLogo.svg"
          alt="Payverge Logo"
          width={128}
          height={128}
          className="h-auto mb-6 mx-auto hover:scale-110 transition-transform duration-300"
        />
        <h1 className="text-5xl font-extrabold mb-6 animate-gradient bg-gradient-text">
          Maintenance Mode
        </h1>
        <p className="mb-8 text-lg text-gray-700 animate-slide-up">
          Our site is currently under maintenance. We will be back soon!
        </p>
        <div className="flex flex-col gap-4 animate-fade-in-delayed">
          <a
            href="https://yourapp.com"
            className="text-blue-500 hover:text-blue-700 hover:translate-x-2 transition-all duration-300"
          >
            Visit our Website
          </a>
          <a
            href="https://docs.yourapp.com"
            className="text-blue-500 hover:text-blue-700 hover:translate-x-2 transition-all duration-300"
          >
            Check our Documentation
          </a>
          <a
            href="https://links.yourapp.com"
            className="text-blue-500 hover:text-blue-700 hover:translate-x-2 transition-all duration-300"
          >
            Explore all our Links
          </a>
        </div>
      </div>
      <style jsx global>{`
        @keyframes gradient {
          0% {
            background-position: 0% 50%;
          }
          50% {
            background-position: 100% 50%;
          }
          100% {
            background-position: 0% 50%;
          }
        }

        .bg-gradient-text {
          background: linear-gradient(270deg, #0858FE, #00A3FF, #0858FE);
          background-size: 200% auto;
          color: transparent;
          -webkit-background-clip: text;
          background-clip: text;
        }

        .animate-gradient {
          animation: gradient 3s ease infinite;
        }

        @keyframes fadeIn {
          from {
            opacity: 0;
          }
          to {
            opacity: 1;
          }
        }

        @keyframes slideUp {
          from {
            opacity: 0;
            transform: translateY(20px);
          }
          to {
            opacity: 1;
            transform: translateY(0);
          }
        }

        .animate-fade-in-delayed {
          opacity: 0;
          animation: fadeIn 1s ease-out 0.5s forwards;
        }

        .animate-slide-up {
          animation: slideUp 1s ease-out;
        }
      `}</style>
    </div>
  );
}
