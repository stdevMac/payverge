import { PayvergeManagement } from "@/components/admin/PayvergeManagement";

type Props = {};

const page = (props: Props) => {
  return (
    <div className="min-h-screen bg-gradient-to-br from-background to-default-50">
      <PayvergeManagement />
    </div>
  );
};

export default page;
