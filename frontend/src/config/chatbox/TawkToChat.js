// TawkToChat.js
"use client";
import { useEffect } from 'react';

const TawkToChat = () => {
    useEffect(() => {
        // Check if the script is already added
        if (window.Tawk_API) return;

        // Create Tawk_API and Tawk_LoadStart variables
        window.Tawk_API = window.Tawk_API || {};
        window.Tawk_LoadStart = new Date();

        // Create a script element and set its attributes
        const s1 = document.createElement('script');
        s1.async = true;
        s1.src = 'https://embed.tawk.to/6714c45a4304e3196ad4afd3/1iakhu0s1';
        s1.charset = 'UTF-8';
        s1.setAttribute('crossorigin', '*');

        // Insert the script before the first script tag
        const s0 = document.getElementsByTagName('script')[0];
        s0.parentNode.insertBefore(s1, s0);
    }, []);

    return null;
};

export default TawkToChat;