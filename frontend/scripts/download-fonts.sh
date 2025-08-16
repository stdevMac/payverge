#!/bin/bash

# Create fonts directory if it doesn't exist
mkdir -p public/fonts

# Download Inter fonts
curl -L -o public/fonts/Inter-Regular.woff2 "https://fonts.gstatic.com/s/inter/v13/UcC73FwrK3iLTeHuS_fvQtMwCp50KnMa1ZL7.woff2"
curl -L -o public/fonts/Inter-Medium.woff2 "https://fonts.gstatic.com/s/inter/v13/UcC73FwrK3iLTeHuS_fvQtMwCp50KnMa2JL7.woff2"
curl -L -o public/fonts/Inter-SemiBold.woff2 "https://fonts.gstatic.com/s/inter/v13/UcC73FwrK3iLTeHuS_fvQtMwCp50KnMa25L7.woff2"
curl -L -o public/fonts/Inter-Bold.woff2 "https://fonts.gstatic.com/s/inter/v13/UcC73FwrK3iLTeHuS_fvQtMwCp50KnMa2pL7.woff2"

# Download Poppins fonts
curl -L -o public/fonts/Poppins-Regular.woff2 "https://fonts.gstatic.com/s/poppins/v20/pxiEyp8kv8JHgFVrJJfecg.woff2"
curl -L -o public/fonts/Poppins-Bold.woff2 "https://fonts.gstatic.com/s/poppins/v20/pxiByp8kv8JHgFVrLCz7Z1xlFQ.woff2"
curl -L -o public/fonts/Poppins-ExtraBold.woff2 "https://fonts.gstatic.com/s/poppins/v20/pxiByp8kv8JHgFVrLDD4Z1xlFQ.woff2"
