// src/api/index.ts
import { axiosInstance } from "../../tools/instance";


export const uploadFileToS3 = async (
    file: File,
    name: string,
    folder: string,
): Promise<string> => {
    return baseFileToS3(file, name, folder, false)
}

export const uploadProtectedFileToS3 = async (
    file: File,
    name: string,
    folder: string,
): Promise<string> => {
    return baseFileToS3(file, name, folder, true)
}

export const baseFileToS3 = async (
    file: File,
    name: string,
    folder: string,
    protectedFile: boolean,
): Promise<string> => {
    try {
        const formData = new FormData();
        formData.append('file', file);
        formData.append('name', name);
        if (folder) {
            formData.append('folder', folder);
        }

        const response = await axiosInstance.post('/inside/upload' + (protectedFile ? '_protected' : ''), formData, {
            headers: {
                'Content-Type': 'multipart/form-data',
            },
        });

        if (response.status === 200) {
            return response.data.location;
        }

        throw new Error('Failed to upload file');
    } catch (error) {
        console.error("Error uploading file", error);
        throw error;
    }
};

export const getRandomName = (): string => {
    return Math.random().toString(36).slice(2, 10);
};
