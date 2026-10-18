"""Telegram Bot API method catalog used by the PTB compatibility probe."""

from __future__ import annotations


# The server keeps kickChatMember as a compatibility alias. The tuple below
# is the 185-method official surface used by the smoke report.
OFFICIAL_METHODS: tuple[str, ...] = (
    "getMe", "getFile", "getUpdates", "setWebhook", "deleteWebhook", "getWebhookInfo",
    "sendMessage", "sendPhoto", "sendDocument", "sendVideo", "sendVideoNote", "sendSticker",
    "sendAudio", "sendVoice", "sendAnimation", "sendLocation", "sendVenue", "sendContact",
    "sendPoll", "sendDice", "sendChatAction", "sendGame", "sendInvoice", "createInvoiceLink",
    "getMyStarBalance", "getStarTransactions", "refundStarPayment", "editUserStarSubscription",
    "sendMediaGroup", "forwardMessage", "copyMessage", "forwardMessages", "copyMessages",
    "getStickerSet", "createNewStickerSet", "addStickerToSet", "setStickerPositionInSet",
    "deleteStickerFromSet", "replaceStickerInSet", "setStickerEmojiList", "setStickerKeywords",
    "setStickerMaskPosition", "setStickerSetTitle", "setStickerSetThumbnail",
    "setCustomEmojiStickerSetThumbnail", "deleteStickerSet", "getChat", "getUserProfilePhotos",
    "getUserProfileAudios", "getCustomEmojiStickers", "getForumTopicIconStickers",
    "getUserPersonalChatMessages", "setChatPhoto", "deleteChatPhoto", "getChatAdministrators",
    "getChatMemberCount", "getChatMember", "exportChatInviteLink", "createChatInviteLink",
    "editChatInviteLink", "revokeChatInviteLink", "approveChatJoinRequest", "declineChatJoinRequest",
    "createForumTopic", "editForumTopic", "closeForumTopic", "reopenForumTopic",
    "hideGeneralForumTopic", "unhideGeneralForumTopic", "closeGeneralForumTopic",
    "editGeneralForumTopic", "reopenGeneralForumTopic", "unpinAllGeneralForumTopicMessages",
    "deleteForumTopic", "unpinAllForumTopicMessages", "banChatMember", "banChatSenderChat",
    "unbanChatSenderChat", "unbanChatMember", "restrictChatMember", "promoteChatMember",
    "setChatAdministratorCustomTitle", "setChatPermissions", "setChatTitle", "setChatDescription",
    "pinChatMessage", "unpinChatMessage", "unpinAllChatMessages", "leaveChat", "getMyCommands",
    "setMyCommands", "deleteMyCommands", "setChatMenuButton", "getChatMenuButton", "setMyName",
    "getMyName", "setMyDescription", "getMyDescription", "setMyShortDescription",
    "getMyShortDescription", "getMyDefaultAdministratorRights", "setMyDefaultAdministratorRights",
    "setMyProfilePhoto", "removeMyProfilePhoto", "editMessageText", "editMessageCaption",
    "editMessageMedia", "editMessageReplyMarkup", "editMessageLiveLocation", "stopMessageLiveLocation",
    "deleteMessage", "deleteMessages", "deleteMessageReaction", "deleteAllMessageReactions",
    "setMessageReaction", "setGameScore", "getGameHighScores", "getBusinessConnection",
    "readBusinessMessage", "deleteBusinessMessages", "answerCallbackQuery", "answerInlineQuery",
    "answerWebAppQuery", "answerShippingQuery", "answerPreCheckoutQuery", "logOut", "close",
    "sendLivePhoto", "sendPaidMedia", "sendChecklist", "sendMessageDraft", "sendRichMessage",
    "sendRichMessageDraft", "setUserEmojiStatus", "setChatMemberTag", "createChatSubscriptionInviteLink",
    "editChatSubscriptionInviteLink", "answerChatJoinRequestQuery", "sendChatJoinRequestWebApp",
    "setChatStickerSet", "deleteChatStickerSet", "answerGuestQuery", "getUserChatBoosts",
    "getManagedBotToken", "replaceManagedBotToken", "getManagedBotAccessSettings",
    "setManagedBotAccessSettings", "getAvailableGifts", "sendGift", "giftPremiumSubscription",
    "verifyUser", "verifyChat", "removeUserVerification", "removeChatVerification",
    "setBusinessAccountName", "setBusinessAccountUsername", "setBusinessAccountBio",
    "setBusinessAccountProfilePhoto", "removeBusinessAccountProfilePhoto",
    "setBusinessAccountGiftSettings", "getBusinessAccountStarBalance", "transferBusinessAccountStars",
    "getBusinessAccountGifts", "getUserGifts", "getChatGifts", "convertGiftToStars", "upgradeGift",
    "transferGift", "postStory", "repostStory", "editStory", "deleteStory", "savePreparedInlineMessage",
    "savePreparedKeyboardButton", "editMessageChecklist", "stopPoll", "editEphemeralMessageText",
    "editEphemeralMessageMedia", "editEphemeralMessageCaption", "editEphemeralMessageReplyMarkup",
    "approveSuggestedPost", "declineSuggestedPost", "deleteEphemeralMessage", "uploadStickerFile",
    "setPassportDataErrors",
)


COMPATIBILITY_ALIASES: tuple[str, ...] = ("kickChatMember",)

if len(OFFICIAL_METHODS) != 185:
    raise RuntimeError(f"expected 185 official methods, found {len(OFFICIAL_METHODS)}")


def snake_case(name: str) -> str:
    """Convert a Bot API camelCase method to PTB's snake_case spelling."""

    result: list[str] = []
    for index, char in enumerate(name):
        if char.isupper() and index:
            result.append("_")
        result.append(char.lower())
    return "".join(result)

