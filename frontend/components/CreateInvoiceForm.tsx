'use client';

import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { useAccount, useContractWrite, usePrepareContractWrite } from 'wagmi';
import { toast } from 'react-hot-toast';
import { Calendar, DollarSign, Mail, User, FileText, MessageSquare } from 'lucide-react';
import { invoiceAPI, CreateInvoiceRequest } from '../lib/api';
import { INVOICE_CONTRACT_ABI, getContractAddresses, parseUSDC } from '../lib/contract';

interface FormData {
  title: string;
  description: string;
  amount: string;
  creatorName: string;
  payerName: string;
  payerEmail: string;
  dueDate: string;
}

export function CreateInvoiceForm() {
  const { address } = useAccount();
  const [isCreating, setIsCreating] = useState(false);
  
  const {
    register,
    handleSubmit,
    formState: { errors },
    reset,
  } = useForm<FormData>();

  const onSubmit = async (data: FormData) => {
    if (!address) {
      toast.error('Please connect your wallet');
      return;
    }

    setIsCreating(true);
    
    try {
      // Create invoice in backend first
      const invoiceData: CreateInvoiceRequest = {
        creator: address,
        creator_name: data.creatorName || undefined,
        title: data.title,
        description: data.description || undefined,
        amount: parseFloat(data.amount),
        payer_name: data.payerName || undefined,
        payer_email: data.payerEmail || undefined,
        due_date: data.dueDate || undefined,
      };

      const invoice = await invoiceAPI.create(invoiceData);
      
      toast.success('Invoice created successfully!');
      
      // Reset form
      reset();
      
      // Show invoice details
      toast.success(
        `Invoice #${invoice.invoice_id} created! Share this link: ${invoice.payment_link}`,
        { duration: 8000 }
      );
      
    } catch (error: any) {
      console.error('Error creating invoice:', error);
      toast.error(error.response?.data?.error || 'Failed to create invoice');
    } finally {
      setIsCreating(false);
    }
  };

  return (
    <div className="card">
      <div className="mb-6">
        <h2 className="text-2xl font-bold text-gray-900 mb-2">
          Create New Invoice
        </h2>
        <p className="text-gray-600">
          Fill out the details below to create a professional USDC invoice
        </p>
      </div>

      <form onSubmit={handleSubmit(onSubmit)} className="space-y-6">
        {/* Invoice Details */}
        <div className="space-y-4">
          <h3 className="text-lg font-medium text-gray-900 flex items-center">
            <FileText className="h-5 w-5 mr-2" />
            Invoice Details
          </h3>
          
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">
              Invoice Title *
            </label>
            <input
              type="text"
              className="input"
              placeholder="e.g., Website Development Services"
              {...register('title', { required: 'Title is required' })}
            />
            {errors.title && (
              <p className="mt-1 text-sm text-red-600">{errors.title.message}</p>
            )}
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">
              Description
            </label>
            <textarea
              rows={3}
              className="input"
              placeholder="Describe the work or services provided..."
              {...register('description')}
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">
              Amount (USDC) *
            </label>
            <div className="relative">
              <DollarSign className="absolute left-3 top-1/2 transform -translate-y-1/2 h-4 w-4 text-gray-400" />
              <input
                type="number"
                step="0.01"
                min="0.01"
                className="input pl-10"
                placeholder="0.00"
                {...register('amount', { 
                  required: 'Amount is required',
                  min: { value: 0.01, message: 'Amount must be at least $0.01' }
                })}
              />
            </div>
            {errors.amount && (
              <p className="mt-1 text-sm text-red-600">{errors.amount.message}</p>
            )}
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">
              Due Date (Optional)
            </label>
            <div className="relative">
              <Calendar className="absolute left-3 top-1/2 transform -translate-y-1/2 h-4 w-4 text-gray-400" />
              <input
                type="date"
                className="input pl-10"
                {...register('dueDate')}
              />
            </div>
          </div>
        </div>

        {/* Creator Details */}
        <div className="space-y-4">
          <h3 className="text-lg font-medium text-gray-900 flex items-center">
            <User className="h-5 w-5 mr-2" />
            Your Details
          </h3>
          
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">
              Your Name (Optional)
            </label>
            <input
              type="text"
              className="input"
              placeholder="Your name or business name"
              {...register('creatorName')}
            />
          </div>
        </div>

        {/* Payer Details */}
        <div className="space-y-4">
          <h3 className="text-lg font-medium text-gray-900 flex items-center">
            <MessageSquare className="h-5 w-5 mr-2" />
            Client Details (Optional)
          </h3>
          
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">
              Client Name
            </label>
            <input
              type="text"
              className="input"
              placeholder="Client or company name"
              {...register('payerName')}
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">
              Client Email
            </label>
            <div className="relative">
              <Mail className="absolute left-3 top-1/2 transform -translate-y-1/2 h-4 w-4 text-gray-400" />
              <input
                type="email"
                className="input pl-10"
                placeholder="client@example.com"
                {...register('payerEmail')}
              />
            </div>
            <p className="mt-1 text-sm text-gray-500">
              If provided, we'll send the invoice and payment notifications to this email
            </p>
          </div>
        </div>

        {/* Submit Button */}
        <div className="pt-4">
          <button
            type="submit"
            disabled={isCreating}
            className="btn-primary w-full py-3 text-base font-medium"
          >
            {isCreating ? 'Creating Invoice...' : 'Create Invoice'}
          </button>
        </div>

        {/* Fee Notice */}
        <div className="bg-blue-50 border border-blue-200 rounded-lg p-4">
          <div className="flex items-start">
            <div className="flex-shrink-0">
              <DollarSign className="h-5 w-5 text-blue-600" />
            </div>
            <div className="ml-3">
              <h4 className="text-sm font-medium text-blue-900">
                Platform Fee: 1%
              </h4>
              <p className="text-sm text-blue-700 mt-1">
                When your client pays, 99% goes directly to your wallet and 1% covers platform costs.
                Much lower than traditional payment processors!
              </p>
            </div>
          </div>
        </div>
      </form>
    </div>
  );
}
