import { PaymentPerson } from '../components/splitting/PaymentSummary';

export interface ReceiptData {
  billId: string;
  billNumber: string;
  businessName: string;
  businessAddress?: string;
  tableNumber?: string;
  subtotal: number;
  taxAmount: number;
  serviceFeeAmount: number;
  totalBillAmount: number;
  splitMethod: 'equal' | 'custom' | 'items';
  people: PaymentPerson[];
  generatedAt: string;
  items?: Array<{
    id: string;
    name: string;
    price: number;
    quantity: number;
    subtotal: number;
  }>;
}

export class ReceiptGenerator {
  private static formatCurrency(amount: number): string {
    return `$${amount.toFixed(2)}`;
  }

  private static formatDate(date: string | Date): string {
    return new Date(date).toLocaleString();
  }

  /**
   * Generate HTML receipt for split bill
   */
  static generateHTMLReceipt(data: ReceiptData): string {
    const totalPaid = data.people.reduce((sum, person) => 
      person.status === 'completed' ? sum + person.totalAmount : sum, 0
    );
    
    const totalTips = data.people.reduce((sum, person) => 
      person.status === 'completed' ? sum + person.tipAmount : sum, 0
    );

    const completedPayments = data.people.filter(p => p.status === 'completed').length;

    return `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Split Bill Receipt - ${data.billNumber}</title>
    <style>
        body {
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
            max-width: 600px;
            margin: 0 auto;
            padding: 20px;
            background-color: #f8f9fa;
        }
        .receipt {
            background: white;
            border-radius: 12px;
            box-shadow: 0 4px 6px rgba(0, 0, 0, 0.1);
            overflow: hidden;
        }
        .header {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            padding: 30px 20px;
            text-align: center;
        }
        .header h1 {
            margin: 0;
            font-size: 28px;
            font-weight: 600;
        }
        .header p {
            margin: 5px 0 0 0;
            opacity: 0.9;
            font-size: 16px;
        }
        .content {
            padding: 30px;
        }
        .bill-info {
            display: grid;
            grid-template-columns: 1fr 1fr;
            gap: 20px;
            margin-bottom: 30px;
            padding: 20px;
            background: #f8f9fa;
            border-radius: 8px;
        }
        .info-item {
            text-align: center;
        }
        .info-label {
            font-size: 12px;
            color: #6c757d;
            text-transform: uppercase;
            letter-spacing: 0.5px;
            margin-bottom: 5px;
        }
        .info-value {
            font-size: 16px;
            font-weight: 600;
            color: #212529;
        }
        .section {
            margin-bottom: 30px;
        }
        .section-title {
            font-size: 18px;
            font-weight: 600;
            color: #212529;
            margin-bottom: 15px;
            padding-bottom: 8px;
            border-bottom: 2px solid #e9ecef;
        }
        .bill-summary {
            background: #f8f9fa;
            padding: 20px;
            border-radius: 8px;
        }
        .summary-row {
            display: flex;
            justify-content: space-between;
            margin-bottom: 8px;
            font-size: 14px;
        }
        .summary-row.total {
            font-weight: 600;
            font-size: 16px;
            padding-top: 8px;
            border-top: 1px solid #dee2e6;
            margin-top: 8px;
        }
        .people-grid {
            display: grid;
            gap: 15px;
        }
        .person-card {
            border: 1px solid #e9ecef;
            border-radius: 8px;
            padding: 15px;
            background: white;
        }
        .person-card.completed {
            border-color: #28a745;
            background: #f8fff9;
        }
        .person-card.pending {
            border-color: #ffc107;
            background: #fffbf0;
        }
        .person-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 10px;
        }
        .person-name {
            font-weight: 600;
            font-size: 16px;
        }
        .status-badge {
            padding: 4px 8px;
            border-radius: 12px;
            font-size: 12px;
            font-weight: 500;
            text-transform: uppercase;
        }
        .status-completed {
            background: #d4edda;
            color: #155724;
        }
        .status-pending {
            background: #fff3cd;
            color: #856404;
        }
        .person-details {
            font-size: 14px;
            color: #6c757d;
        }
        .person-amount {
            font-size: 18px;
            font-weight: 600;
            color: #212529;
            margin-top: 8px;
        }
        .items-list {
            margin-top: 10px;
            padding-top: 10px;
            border-top: 1px solid #e9ecef;
        }
        .item {
            display: flex;
            justify-content: space-between;
            font-size: 12px;
            margin-bottom: 4px;
            color: #6c757d;
        }
        .footer {
            background: #f8f9fa;
            padding: 20px;
            text-align: center;
            font-size: 12px;
            color: #6c757d;
        }
        .progress-summary {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            padding: 20px;
            border-radius: 8px;
            margin-bottom: 20px;
            text-align: center;
        }
        .progress-title {
            font-size: 16px;
            margin-bottom: 10px;
        }
        .progress-stats {
            display: grid;
            grid-template-columns: repeat(3, 1fr);
            gap: 15px;
            margin-top: 15px;
        }
        .progress-stat {
            text-align: center;
        }
        .progress-number {
            font-size: 24px;
            font-weight: 600;
        }
        .progress-label {
            font-size: 12px;
            opacity: 0.9;
        }
        @media print {
            body { background: white; }
            .receipt { box-shadow: none; }
        }
    </style>
</head>
<body>
    <div class="receipt">
        <div class="header">
            <h1>Split Bill Receipt</h1>
            <p>Bill #${data.billNumber} • ${data.businessName}</p>
        </div>
        
        <div class="content">
            <div class="bill-info">
                <div class="info-item">
                    <div class="info-label">Split Method</div>
                    <div class="info-value">${data.splitMethod === 'equal' ? 'Equal Split' : 
                      data.splitMethod === 'custom' ? 'Custom Amounts' : 'Item-based Split'}</div>
                </div>
                <div class="info-item">
                    <div class="info-label">Generated</div>
                    <div class="info-value">${this.formatDate(data.generatedAt)}</div>
                </div>
                <div class="info-item">
                    <div class="info-label">Total People</div>
                    <div class="info-value">${data.people.length}</div>
                </div>
                <div class="info-item">
                    <div class="info-label">Payments Complete</div>
                    <div class="info-value">${completedPayments}/${data.people.length}</div>
                </div>
            </div>

            <div class="progress-summary">
                <div class="progress-title">Payment Progress</div>
                <div class="progress-stats">
                    <div class="progress-stat">
                        <div class="progress-number">${this.formatCurrency(totalPaid)}</div>
                        <div class="progress-label">Total Paid</div>
                    </div>
                    <div class="progress-stat">
                        <div class="progress-number">${this.formatCurrency(totalTips)}</div>
                        <div class="progress-label">Tips Added</div>
                    </div>
                    <div class="progress-stat">
                        <div class="progress-number">${Math.round((completedPayments / data.people.length) * 100)}%</div>
                        <div class="progress-label">Complete</div>
                    </div>
                </div>
            </div>

            <div class="section">
                <div class="section-title">Bill Summary</div>
                <div class="bill-summary">
                    <div class="summary-row">
                        <span>Subtotal:</span>
                        <span>${this.formatCurrency(data.subtotal)}</span>
                    </div>
                    ${data.taxAmount > 0 ? `
                    <div class="summary-row">
                        <span>Tax:</span>
                        <span>${this.formatCurrency(data.taxAmount)}</span>
                    </div>` : ''}
                    ${data.serviceFeeAmount > 0 ? `
                    <div class="summary-row">
                        <span>Service Fee:</span>
                        <span>${this.formatCurrency(data.serviceFeeAmount)}</span>
                    </div>` : ''}
                    <div class="summary-row total">
                        <span>Bill Total:</span>
                        <span>${this.formatCurrency(data.totalBillAmount)}</span>
                    </div>
                    <div class="summary-row">
                        <span>Tips Added:</span>
                        <span>${this.formatCurrency(totalTips)}</span>
                    </div>
                    <div class="summary-row total">
                        <span>Grand Total:</span>
                        <span>${this.formatCurrency(data.totalBillAmount + totalTips)}</span>
                    </div>
                </div>
            </div>

            <div class="section">
                <div class="section-title">Individual Payments</div>
                <div class="people-grid">
                    ${data.people.map(person => `
                    <div class="person-card ${person.status}">
                        <div class="person-header">
                            <div class="person-name">${person.name}</div>
                            <div class="status-badge status-${person.status}">
                                ${person.status === 'completed' ? 'Paid' : 
                                  person.status === 'processing' ? 'Processing' : 'Pending'}
                            </div>
                        </div>
                        <div class="person-details">
                            Bill Amount: ${this.formatCurrency(person.amount)} • 
                            Tip: ${this.formatCurrency(person.tipAmount)}
                        </div>
                        <div class="person-amount">
                            Total: ${this.formatCurrency(person.totalAmount)}
                        </div>
                        ${person.items && person.items.length > 0 ? `
                        <div class="items-list">
                            ${person.items.map(item => `
                            <div class="item">
                                <span>${item.name} × ${item.quantity}</span>
                                <span>${this.formatCurrency(item.price * item.quantity)}</span>
                            </div>
                            `).join('')}
                        </div>` : ''}
                        ${person.transactionHash ? `
                        <div class="person-details" style="margin-top: 8px; font-family: monospace; font-size: 11px;">
                            TX: ${person.transactionHash.substring(0, 20)}...
                        </div>` : ''}
                    </div>
                    `).join('')}
                </div>
            </div>
        </div>
        
        <div class="footer">
            <p>Generated by Payverge • Blockchain-powered hospitality payments</p>
            <p>This receipt serves as a record of the split bill arrangement and payment status.</p>
        </div>
    </div>
</body>
</html>`;
  }

  /**
   * Generate and download HTML receipt
   */
  static downloadHTMLReceipt(data: ReceiptData): void {
    const html = this.generateHTMLReceipt(data);
    const blob = new Blob([html], { type: 'text/html' });
    const url = URL.createObjectURL(blob);
    
    const a = document.createElement('a');
    a.href = url;
    a.download = `split-bill-receipt-${data.billNumber}-${Date.now()}.html`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  }

  /**
   * Generate printable receipt (opens print dialog)
   */
  static printReceipt(data: ReceiptData): void {
    const html = this.generateHTMLReceipt(data);
    const printWindow = window.open('', '_blank');
    if (printWindow) {
      printWindow.document.write(html);
      printWindow.document.close();
      printWindow.focus();
      printWindow.print();
    }
  }

  /**
   * Generate shareable receipt summary text
   */
  static generateShareableText(data: ReceiptData): string {
    const totalPaid = data.people.reduce((sum, person) => 
      person.status === 'completed' ? sum + person.totalAmount : sum, 0
    );
    
    const completedPayments = data.people.filter(p => p.status === 'completed').length;
    
    return `🧾 Split Bill Receipt - ${data.businessName}

📋 Bill #${data.billNumber}
💰 Total: ${this.formatCurrency(data.totalBillAmount)}
👥 Split among ${data.people.length} people
✅ ${completedPayments}/${data.people.length} payments complete

💳 Paid so far: ${this.formatCurrency(totalPaid)}
⏳ Remaining: ${this.formatCurrency((data.totalBillAmount + data.people.reduce((sum, p) => sum + p.tipAmount, 0)) - totalPaid)}

Individual amounts:
${data.people.map(person => 
  `${person.status === 'completed' ? '✅' : '⏳'} ${person.name}: ${this.formatCurrency(person.totalAmount)}`
).join('\n')}

Generated by Payverge 🚀`;
  }

  /**
   * Copy shareable text to clipboard
   */
  static async copyShareableText(data: ReceiptData): Promise<boolean> {
    try {
      const text = this.generateShareableText(data);
      await navigator.clipboard.writeText(text);
      return true;
    } catch (error) {
      console.error('Failed to copy to clipboard:', error);
      return false;
    }
  }
}
