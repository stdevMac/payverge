// Argentine (es-AR) operator-dashboard OVERRIDE layer.
//
// AUTO-ASSEMBLED from the Rioplatense translation pass (see
// docs/i18n/rioplatense-style-guide.md). This is NOT a full translation — it is
// deep-merged OVER the neutral `es` message base by getTranslation /
// SimpleTranslationProvider (`deepMerge(esTree, esArOverrides)`), so each file
// should hold ONLY the keys whose value differs in Argentine Spanish
// (voseo: tenés/podés/configurá/elegí; vocab: mozo, celular, "cuenta" not
// "factura" for the open bill). Every key NOT present here inherits its `es`
// value automatically — es-AR can never have a missing-key gap relative to es.
//
// Batch F4c: ~150 leaf keys currently identical to es are inert under merge
// (deadweight, not harmful). Do not bulk-prune without re-running the
// Rioplatense assembler; prefer deleting only verified identical leaves in a
// dedicated hygiene PR. Namespaces with zero Argentine divergences are
// intentionally absent.
//
// To regenerate after editing es: re-run the operator es-AR pass and the
// assembler.

import account from "./account.json";
import aiMenuOnboarding from "./aiMenuOnboarding.json";
import cookies from "./cookies.json";
import aiWaiterDashboard from "./aiWaiterDashboard.json";
import aiWaiterToggle from "./aiWaiterToggle.json";
import alternativePaymentManager from "./alternativePaymentManager.json";
import authModal from "./authModal.json";
import bannerImageUploader from "./bannerImageUploader.json";
import billCreator from "./billCreator.json";
import billManager from "./billManager.json";
import businessDashboard from "./businessDashboard.json";
import businessPage from "./businessPage.json";
import businessRegister from "./businessRegister.json";
import businessSettings from "./businessSettings.json";
import common from "./common.json";
import crmToggle from "./crmToggle.json";
import currencySettings from "./currencySettings.json";
import dashboard from "./dashboard.json";
import dashboardApprovals from "./dashboardApprovals.json";
import dashboardChat from "./dashboardChat.json";
import dashboardKiosk from "./dashboardKiosk.json";
import dashboardSchedule from "./dashboardSchedule.json";
import dashboardScheduleSettings from "./dashboardScheduleSettings.json";
import dashboardSetup from "./dashboardSetup.json";
import dashboardTimesheets from "./dashboardTimesheets.json";
import deliverySettings from "./deliverySettings.json";
import deliveryToggle from "./deliveryToggle.json";
import directorConsole from "./directorConsole.json";
import marketingDashboard from "./marketingDashboard.json";
import fiscal from "./fiscal.json";
import forgotPassword from "./forgotPassword.json";
import googleBusinessSearch from "./googleBusinessSearch.json";
import imageUpload from "./imageUpload.json";
import kitchenOrdersToggle from "./kitchenOrdersToggle.json";
import legal from "./legal.json";
import navigation from "./navigation.json";
import notFound from "./notFound.json";
import opsAssistant from "./opsAssistant.json";
import paymentProcessor from "./paymentProcessor.json";
import mercadoPagoPoint from "./mercadoPagoPoint.json";
import mercadoPagoQR from "./mercadoPagoQR.json";
import pluginsManagement from "./pluginsManagement.json";
import printers from "./printers.json";
import resetPassword from "./resetPassword.json";
import scan from "./scan.json";
import staffAvailability from "./staffAvailability.json";
import staffChat from "./staffChat.json";
import staffEngagement from "./staffEngagement.json";
import staffLogbook from "./staffLogbook.json";
import staffNotifications from "./staffNotifications.json";
import staffHours from "./staffHours.json";
import staffToday from "./staffToday.json";
import staffHome from "./staffHome.json";
import staffSchedule from "./staffSchedule.json";
import staffTimeclock from "./staffTimeclock.json";
import staffCoverage from "./staffCoverage.json";
import staffInvitation from "./staffInvitation.json";
import staffLogin from "./staffLogin.json";
import businessLock from "./businessLock.json";
import verifyEmail from "./verifyEmail.json";
import spacesTables from "./spacesTables.json";
import pwa from "./pwa.json";
import urlState from "./urlState.json";

const esArOverrides: Record<string, Record<string, unknown>> = {
  account,
  aiMenuOnboarding,
  cookies,
  aiWaiterDashboard,
  aiWaiterToggle,
  alternativePaymentManager,
  authModal,
  bannerImageUploader,
  billCreator,
  billManager,
  businessDashboard,
  businessPage,
  businessRegister,
  businessSettings,
  common,
  crmToggle,
  currencySettings,
  dashboard,
  dashboardApprovals,
  dashboardChat,
  dashboardKiosk,
  dashboardSchedule,
  dashboardScheduleSettings,
  dashboardSetup,
  dashboardTimesheets,
  deliverySettings,
  deliveryToggle,
  directorConsole,
  marketingDashboard,
  fiscal,
  forgotPassword,
  googleBusinessSearch,
  imageUpload,
  kitchenOrdersToggle,
  legal,
  navigation,
  notFound,
  opsAssistant,
  paymentProcessor,
  mercadoPagoPoint,
  mercadoPagoQR,
  pluginsManagement,
  printers,
  resetPassword,
  scan,
  staffAvailability,
  staffChat,
  staffEngagement,
  staffLogbook,
  staffNotifications,
  staffHours,
  staffToday,
  staffHome,
  staffSchedule,
  staffTimeclock,
  staffCoverage,
  staffInvitation,
  staffLogin,
  businessLock,
  verifyEmail,
  spacesTables,
  pwa,
  urlState,
};

export default esArOverrides;
