'use client';

import React, { useRef } from 'react';
import { Button } from '@nextui-org/react';
import Image from 'next/image';
import html2canvas from 'html2canvas';
import jsPDF from 'jspdf';

interface ReceiptItem {
  name: string;
  quantity: number;
  subtotal: number;
}

interface ReceiptData {
  business: {
    name: string;
    logo?: string;
    address?: {
      street?: string;
      city?: string;
      state?: string;
      postal_code?: string;
      country?: string;
    };
    phone?: string;
    email?: string;
  };
  bill: {
    bill_number: string;
    created_at: string;
    subtotal: number;
    tax_amount: number;
    service_fee_amount: number;
    total_amount: number;
  };
  table: {
    name: string;
  };
  items: ReceiptItem[];
  paymentDetails?: {
    totalPaid: number;
    tipAmount: number;
    paymentMethod: string;
    transactionId?: string;
  };
}

interface ReceiptGeneratorProps {
  data: ReceiptData;
  onDownloadReceipt?: (pdfBlob: Blob) => void;
  className?: string;
}

export default function ReceiptGenerator({ 
  data, 
  onDownloadReceipt, 
  className = "" 
}: ReceiptGeneratorProps) {
  const receiptRef = useRef<HTMLDivElement>(null);

  const generatePDF = async (): Promise<Blob> => {
    if (!receiptRef.current) throw new Error('Receipt ref not available');

    const canvas = await html2canvas(receiptRef.current, {
      scale: 2,
      useCORS: true,
      allowTaint: true,
      backgroundColor: '#ffffff',
    });

    const imgData = canvas.toDataURL('image/png');
    const pdf = new jsPDF('p', 'mm', 'a4');
    
    // Calculate dimensions to fit on single page with margins
    const pageWidth = 210; // A4 width in mm
    const pageHeight = 297; // A4 height in mm
    const margin = 10; // 10mm margin on all sides
    const maxWidth = pageWidth - (margin * 2);
    const maxHeight = pageHeight - (margin * 2);
    
    // Calculate the aspect ratio and fit the image
    const imgAspectRatio = canvas.width / canvas.height;
    let imgWidth = maxWidth;
    let imgHeight = imgWidth / imgAspectRatio;
    
    // If height exceeds page, scale down based on height
    if (imgHeight > maxHeight) {
      imgHeight = maxHeight;
      imgWidth = imgHeight * imgAspectRatio;
    }
    
    // Center the image on the page
    const xPosition = (pageWidth - imgWidth) / 2;
    const yPosition = (pageHeight - imgHeight) / 2;

    pdf.addImage(imgData, 'PNG', xPosition, yPosition, imgWidth, imgHeight);

    return pdf.output('blob');
  };

  const generateImage = async (): Promise<Blob> => {
    if (!receiptRef.current) throw new Error('Receipt ref not available');

    const canvas = await html2canvas(receiptRef.current, {
      scale: 2,
      useCORS: true,
      allowTaint: true,
      backgroundColor: '#ffffff',
    });

    return new Promise((resolve) => {
      canvas.toBlob((blob) => {
        if (blob) resolve(blob);
      }, 'image/png');
    });
  };

  const handleDownloadReceipt = async () => {
    try {
      const pdfBlob = await generatePDF();
      if (onDownloadReceipt) {
        onDownloadReceipt(pdfBlob);
      } else {
        // Fallback: create download link
        const url = URL.createObjectURL(pdfBlob);
        const a = document.createElement('a');
        a.href = url;
        a.download = `receipt-${data.bill.bill_number}.pdf`;
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        URL.revokeObjectURL(url);
      }
    } catch (error) {
      console.error('Error generating PDF:', error);
      alert('Error generating receipt. Please try again.');
    }
  };

  const formatDate = (dateString: string) => {
    return new Date(dateString).toLocaleString();
  };

  const formatAddress = (address: any) => {
    if (!address) return '';
    const parts = [
      address.street,
      address.city,
      address.state,
      address.postal_code,
      address.country
    ].filter(Boolean);
    return parts.join(', ');
  };

  return (
    <div className={className}>
      {/* Hidden Receipt for Generation */}
      <div 
        ref={receiptRef} 
        className="bg-white p-6 max-w-md mx-auto"
        style={{ 
          position: 'absolute', 
          left: '-9999px', 
          top: '-9999px',
          width: '384px' // Fixed width for consistent generation
        }}
      >
        {/* Business Header */}
        <div className="text-center mb-4">
          {data.business.logo && (
            <Image 
              src={data.business.logo} 
              alt={data.business.name}
              width={48}
              height={48}
              className="mx-auto mb-2 object-contain"
            />
          )}
          <h1 className="text-xl font-bold text-gray-900 mb-1">{data.business.name}</h1>
          {data.business.address && (
            <p className="text-sm text-gray-600 mb-1">{formatAddress(data.business.address)}</p>
          )}
          {data.business.phone && (
            <p className="text-sm text-gray-600 mb-1">Phone: {data.business.phone}</p>
          )}
          {data.business.email && (
            <p className="text-sm text-gray-600">Email: {data.business.email}</p>
          )}
        </div>

        <div className="border-t border-b border-gray-300 py-3 mb-3">
          <div className="flex justify-between mb-1">
            <span className="font-semibold text-sm">Receipt #:</span>
            <span className="text-sm">{data.bill.bill_number}</span>
          </div>
          <div className="flex justify-between mb-1">
            <span className="font-semibold text-sm">Table:</span>
            <span className="text-sm">{data.table.name}</span>
          </div>
          <div className="flex justify-between mb-1">
            <span className="font-semibold text-sm">Date:</span>
            <span className="text-sm">{formatDate(data.bill.created_at)}</span>
          </div>
          {data.paymentDetails?.transactionId && (
            <div className="flex justify-between">
              <span className="font-semibold text-sm">Transaction ID:</span>
              <span className="text-xs">{data.paymentDetails.transactionId}</span>
            </div>
          )}
        </div>

        {/* Items */}
        <div className="mb-3">
          <h3 className="font-semibold mb-2 text-sm">Order Details</h3>
          {data.items.map((item, index) => (
            <div key={index} className="flex justify-between items-center py-1 border-b border-gray-100">
              <div className="flex-1">
                <div className="font-medium text-sm">{item.name}</div>
                <div className="text-xs text-gray-600">Qty: {item.quantity}</div>
              </div>
              <div className="font-medium text-sm">${item.subtotal.toFixed(2)}</div>
            </div>
          ))}
        </div>

        {/* Totals */}
        <div className="border-t border-gray-300 pt-3 space-y-1">
          <div className="flex justify-between text-sm">
            <span>Subtotal:</span>
            <span>${data.bill.subtotal.toFixed(2)}</span>
          </div>
          {data.bill.tax_amount > 0 && (
            <div className="flex justify-between text-sm">
              <span>Tax:</span>
              <span>${data.bill.tax_amount.toFixed(2)}</span>
            </div>
          )}
          {data.bill.service_fee_amount > 0 && (
            <div className="flex justify-between text-sm">
              <span>Service Fee:</span>
              <span>${data.bill.service_fee_amount.toFixed(2)}</span>
            </div>
          )}
          {data.paymentDetails?.tipAmount && data.paymentDetails.tipAmount > 0 && (
            <div className="flex justify-between text-green-600 text-sm">
              <span>Tip:</span>
              <span>${data.paymentDetails.tipAmount.toFixed(2)}</span>
            </div>
          )}
          <div className="flex justify-between font-bold border-t border-gray-300 pt-2">
            <span>Total Paid:</span>
            <span>${data.paymentDetails?.totalPaid.toFixed(2) || data.bill.total_amount.toFixed(2)}</span>
          </div>
        </div>

        {/* Payment Method */}
        {data.paymentDetails?.paymentMethod && (
          <div className="mt-3 pt-3 border-t border-gray-300">
            <div className="flex justify-between text-sm">
              <span className="font-semibold">Payment Method:</span>
              <span>{data.paymentDetails.paymentMethod}</span>
            </div>
          </div>
        )}

        {/* Footer */}
        <div className="text-center mt-4 pt-3 border-t border-gray-300">
          <p className="font-semibold mb-1">Thank You!</p>
          <p className="text-sm text-gray-600">Thank you for choosing {data.business.name}</p>
          <p className="text-sm text-gray-600">We hope to see you again soon!</p>
          <div className="mt-3 text-xs text-gray-500">
            <p>Powered by Payverge</p>
            <p>Digital Receipt System</p>
          </div>
        </div>
      </div>

      {/* Action Button */}
      <div className="flex justify-center">
        <Button
          size="lg"
          className="bg-blue-600 text-white hover:bg-blue-700 min-w-48"
          startContent={
            <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 10v6m0 0l-3-3m3 3l3-3m2 8H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
            </svg>
          }
          onClick={handleDownloadReceipt}
        >
          Download Receipt
        </Button>
      </div>
    </div>
  );
}
