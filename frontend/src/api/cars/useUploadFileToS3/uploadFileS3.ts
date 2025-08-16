"use client";
import { useState, useCallback } from "react";
import { uploadFileToS3 as uploadToS3, uploadProtectedFileToS3 } from "@/api";
import { downloadContract } from "@/api/users/getContract";

export const useS3Upload = () => {
    const [uploading, setUploading] = useState(false);
    const [error, setError] = useState<Error | null>(null);

    const uploadFileToS3 = useCallback(async (file: File, name: string, folder: string) => {
        setUploading(true);
        setError(null);
        try {
            const url = await uploadToS3(file, name, folder);
            return url;
        } catch (err) {
            setError(err as Error);
            throw err;
        } finally {
            setUploading(false);
        }
    }, []);

    return { uploadFileToS3, uploading, error };
};

export const useContractDownload = () => {
    const [downloading, setDownloading] = useState(false);
    const [error, setError] = useState<Error | null>(null);

    const downloadContractFile = useCallback(async (url: string): Promise<{ blob: Blob; filename: string }> => {
        setDownloading(true);
        setError(null);
        try {
            const response = await downloadContract(url);
            return response;
        } catch (err) {
            setError(err as Error);
            throw err;
        } finally {
            setDownloading(false);
        }
    }, []);

    return { downloadContractFile, downloading, error };
};
export const useS3ProtectedUpload = () => {
    const [uploading, setUploading] = useState(false);
    const [error, setError] = useState<Error | null>(null);

    const uploadFileToS3 = useCallback(async (file: File, name: string, folder: string) => {
        setUploading(true);
        setError(null);
        try {
            const url = await uploadProtectedFileToS3(file, name, folder);
            return url;
        } catch (err) {
            setError(err as Error);
            throw err;
        } finally {
            setUploading(false);
        }
    }, []);

    return { uploadFileToS3, uploading, error };
};
