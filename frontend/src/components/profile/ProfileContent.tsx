import { UserInterface } from "@/interface";
import { Button, Input, Card} from "@nextui-org/react";
import { useState } from "react";

interface ProfileContentProps {
  user: UserInterface;
  formData: UserInterface;
  onUpdateProfile: (data: Partial<UserInterface>) => Promise<boolean>;
  onRefetch: () => Promise<void>;
}

const ProfileContent: React.FC<ProfileContentProps> = ({
  user,
  formData,
  onUpdateProfile,
  onRefetch,
}) => {
  const [isEditing, setIsEditing] = useState(false);
  const [editedUsername, setEditedUsername] = useState(user.username);

  const handleSave = async () => {
    const success = await onUpdateProfile({ username: editedUsername });
    if (success) {
      setIsEditing(false);
      onRefetch();
    }
  };

  const formatCurrency = (value: number) => {
    return new Intl.NumberFormat("en-US", {
      style: "currency",
      currency: "USD",
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    }).format(value);
  };


  return (
    <div className="mt-24 space-y-6">
      {/* Personal Information Card */}
      <Card className="p-6">
        <div className="flex justify-between items-center mb-4">
          <h3 className="text-lg font-semibold">Personal Information</h3>
          <Button
            color="primary"
            size="sm"
            variant="light"
            onClick={() => setIsEditing(!isEditing)}
          >
            {isEditing ? "Cancel" : "Edit"}
          </Button>
        </div>
        <div className="space-y-4">
          {isEditing ? (
            <div className="flex items-center space-x-2">
              <Input
                value={editedUsername}
                onChange={(e) => setEditedUsername(e.target.value)}
                placeholder="Enter username"
                size="sm"
              />
              <Button color="primary" size="sm" onClick={handleSave}>
                Save
              </Button>
            </div>
          ) : (
            <div>
              <p className="text-sm text-gray-500">Username</p>
              <p className="font-medium">{user.username || "Not set"}</p>
            </div>
          )}
          <div>
            <p className="text-sm text-gray-500">Wallet Address</p>
            <p className="font-medium truncate">{user.address}</p>
          </div>
          <div>
            <p className="text-sm text-gray-500">Email</p>
            <p className="font-medium">{user.email || "Not set"}</p>
          </div>
        </div>
      </Card>

      {/* Referral Program */}
      <Card className="p-6">
        <h3 className="text-lg font-semibold mb-4">Referral Program</h3>
        <div className="space-y-4">
          <div className="grid grid-cols-2 gap-4">
            <div>
              <p className="text-sm text-gray-500">Total Referrals</p>
              <p className="font-medium">
                {Object.keys(user.referees || {}).length || 0}
              </p>
            </div>
          </div>
          <div>
            <p className="text-sm text-gray-500 mb-2">Recent Referrals</p>
            <div className="space-y-2">
              {Object.entries(user.referees || {})
                .slice(0, 3)
                .map(([address, amount]) => (
                  <div
                    key={address}
                    className="flex justify-between items-center"
                  >
                    <p className="text-sm truncate w-32">{address}</p>
                    <p className="text-sm font-medium">
                      {formatCurrency(amount)}
                    </p>
                  </div>
                ))}
            </div>
          </div>
        </div>
      </Card>
    </div>
  );
};

export default ProfileContent;
