
import { axiosInstance } from "@/api";

export const downloadContract = async (url: string): Promise<{ blob: Blob; filename: string }> => {
    try {
        const response = await axiosInstance.get('/inside/get_contract', {
            params: { url },
            responseType: 'blob',
            // Ensure axios includes the Content-Disposition header in the response
            headers: {
                Accept: 'application/pdf',
            }
        });

        if (response.status === 200) {
            let filename = 'contract.pdf';
            
            // Get filename from Content-Disposition header
            const contentDisposition = response.headers?.['content-disposition'];
            if (contentDisposition) {
                // First try 'filename=' format
                const filenameMatch = contentDisposition.match(/filename=([^;]+)/i);
                if (filenameMatch?.[1]) {
                    // Remove quotes if present
                    filename = filenameMatch[1].replace(/["']/g, '').trim();
                } else {
                    // Try 'filename*=' format (RFC 5987)
                    const filenameExtMatch = contentDisposition.match(/filename\*=([^']*)'[^']*'([^;]+)/i);
                    if (filenameExtMatch?.[2]) {
                        filename = decodeURIComponent(filenameExtMatch[2].trim());
                    }
                }
            }

            // Fallback to URL if no valid filename found
            if (filename === 'contract.pdf') {
                const urlParts = url.split('/');
                const urlFilename = urlParts[urlParts.length - 1];
                if (urlFilename && urlFilename.length > 0) {
                    filename = decodeURIComponent(urlFilename);
                }
            }

            // Ensure .pdf extension
            if (!filename.toLowerCase().endsWith('.pdf')) {
                filename += '.pdf';
            }

            return {
                blob: new Blob([response.data], { type: 'application/pdf' }),
                filename
            };
        }

        throw new Error('Failed to download contract');
    } catch (error) {
        console.error("Error downloading contract", error);
        throw error;
    }
};

