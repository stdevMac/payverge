import localFont from 'next/font/local';

export const inter = localFont({
    src: [
        {
            path: '../../../public/fonts/Inter-Regular.woff2',
            weight: '400',
            style: 'normal',
        },
        {
            path: '../../../public/fonts/Inter-Medium.woff2',
            weight: '500',
            style: 'normal',
        },
        {
            path: '../../../public/fonts/Inter-SemiBold.woff2',
            weight: '600',
            style: 'normal',
        },
        {
            path: '../../../public/fonts/Inter-Bold.woff2',
            weight: '700',
            style: 'normal',
        },
    ],
    variable: '--font-inter',
    display: 'swap',
});

export const titleFont = localFont({
    src: [
        {
            path: '../../../public/fonts/Poppins-Regular.woff2',
            weight: '400',
            style: 'normal',
        },
        {
            path: '../../../public/fonts/Poppins-Bold.woff2',
            weight: '700',
            style: 'normal',
        },
        {
            path: '../../../public/fonts/Poppins-ExtraBold.woff2',
            weight: '800',
            style: 'normal',
        },
    ],
    variable: '--font-poppins',
    display: 'swap',
});
