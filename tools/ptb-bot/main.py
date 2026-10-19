#!/usr/bin/env python3
"""Small production-shaped python-telegram-bot example for Teamgram."""

from __future__ import annotations

import logging
import os
import inspect
from collections import Counter
from dataclasses import dataclass
from typing import Any

from telegram import (
    InlineKeyboardButton,
    InlineKeyboardMarkup,
    InputMediaDocument,
    InputPaidMediaPhoto,
    KeyboardButton,
    ReplyKeyboardMarkup,
    Update,
    WebAppInfo,
)
from telegram.error import BadRequest
from telegram.ext import (
    Application,
    CallbackQueryHandler,
    CommandHandler,
    ContextTypes,
    MessageHandler,
    filters,
)

from method_catalog import OFFICIAL_METHODS, snake_case


LOGGER = logging.getLogger("teamgram_ptb_bot")
METHOD_PAGE_SIZE = 20
TERMINAL_METHODS = frozenset({"logOut", "close"})
TERMINAL_METHOD_ORDER = ("logOut", "close")
NON_TERMINAL_METHODS = tuple(method for method in OFFICIAL_METHODS if method not in TERMINAL_METHODS)


@dataclass(frozen=True)
class MethodProbeResult:
    method: str
    route: str
    status: str
    detail: str = ""
    category: str = "success"


def _integer_env(name: str, default: int = 0) -> int:
    try:
        return int(os.getenv(name, str(default)))
    except ValueError:
        return default


def _probe_params(method: str) -> dict[str, Any]:
    """Return Telegram-shaped, harmless parameters for one endpoint call."""

    chat_id = _integer_env("BOT_PROBE_CHAT_ID")
    user_id = _integer_env("BOT_PROBE_USER_ID")
    message_id = _integer_env("BOT_PROBE_MESSAGE_ID")
    file_id = os.getenv("BOT_PROBE_FILE_ID", "invalid-file-id")
    business_connection_id = os.getenv(
        "BOT_PROBE_BUSINESS_CONNECTION_ID", "invalid-business-connection"
    )
    owned_gift_id = os.getenv("BOT_PROBE_OWNED_GIFT_ID", "invalid-owned-gift")
    # Keep payment probes fail-closed unless a disposable fixture explicitly
    # supplies a gift id returned by getAvailableGifts.
    gift_id = os.getenv("BOT_PROBE_GIFT_ID", "invalid-gift-id")
    gift_star_count = _integer_env("BOT_PROBE_GIFT_STAR_COUNT", 1)
    params: dict[str, Any] = {}

    if method == "getUpdates":
        # Keep the catalog probe non-blocking so it does not compete with the
        # application's long-polling update consumer on the same bot token.
        return {"limit": 1, "timeout": 0}
    if method == "getFile":
        return {"file_id": file_id}
    if method == "setWebhook":
        return {"url": "http://127.0.0.1:9/teamgram-ptb-probe"}
    if method == "sendMessage":
        return {"chat_id": chat_id, "text": "PTB 185-method smoke probe"}
    if method in {"sendMessageDraft", "sendRichMessage", "sendRichMessageDraft"}:
        params.update(chat_id=chat_id, text="PTB 185-method smoke probe")
    if method in {"sendPhoto", "sendDocument", "sendVideo", "sendVideoNote", "sendSticker",
                  "sendAudio", "sendVoice", "sendAnimation"}:
        field = {
            "sendPhoto": "photo", "sendDocument": "document", "sendVideo": "video",
            "sendVideoNote": "video_note", "sendSticker": "sticker", "sendAudio": "audio",
            "sendVoice": "voice", "sendAnimation": "animation",
        }[method]
        return {"chat_id": chat_id, field: file_id}
    if method == "sendLocation":
        return {"chat_id": chat_id, "latitude": 0.0, "longitude": 0.0}
    if method == "sendVenue":
        return {"chat_id": chat_id, "latitude": 1.0, "longitude": 1.0,
                "title": "PTB probe", "address": "local"}
    if method == "sendContact":
        return {"chat_id": chat_id, "phone_number": "+10000000000", "first_name": "PTB probe"}
    if method == "sendPoll":
        return {"chat_id": chat_id, "question": "PTB probe", "options": ["one", "two"]}
    if method == "sendDice":
        return {"chat_id": chat_id, "emoji": "🎲"}
    if method == "sendChatAction":
        return {"chat_id": chat_id, "action": "typing"}
    if method == "sendGame":
        return {"chat_id": chat_id, "game_short_name": "ptb_probe"}
    if method == "sendInvoice":
        return {"chat_id": chat_id, "title": "PTB probe", "description": "probe",
                "payload": os.getenv("BOT_PROBE_INVOICE_PAYLOAD", "ptb-probe-xtr"),
                "provider_token": os.getenv("BOT_PROBE_PROVIDER_TOKEN", ""),
                "currency": os.getenv("BOT_PROBE_INVOICE_CURRENCY", "XTR"),
                "prices": [{"label": "probe", "amount": 1}]}
    if method == "sendMediaGroup":
        return {"chat_id": chat_id, "media": [
            InputMediaDocument(media=file_id),
            InputMediaDocument(media=file_id),
        ]}
    if method == "sendPaidMedia":
        return {"chat_id": chat_id, "star_count": 1,
                "media": [InputPaidMediaPhoto(
                    media=os.getenv("BOT_PROBE_PHOTO_ID", file_id)
                )]}
    if method == "sendChecklist":
        return {"business_connection_id": business_connection_id, "chat_id": chat_id,
                "checklist": {"title": "PTB probe", "tasks": [{"id": 1, "text": "probe"}]}}
    if method in {"getChat", "getChatAdministrators", "getChatMemberCount", "getChatMember",
                  "exportChatInviteLink", "createChatInviteLink", "editChatInviteLink",
                  "revokeChatInviteLink", "approveChatJoinRequest", "declineChatJoinRequest",
                  "createForumTopic", "editForumTopic", "closeForumTopic", "reopenForumTopic",
                  "hideGeneralForumTopic", "unhideGeneralForumTopic", "closeGeneralForumTopic",
                  "editGeneralForumTopic", "reopenGeneralForumTopic", "deleteForumTopic",
                  "unpinAllForumTopicMessages", "unpinAllGeneralForumTopicMessages", "setChatTitle",
                  "setChatDescription", "deleteChatPhoto", "setChatStickerSet", "deleteChatStickerSet",
                  "getChatGifts", "verifyChat", "removeChatVerification", "getUserGifts"}:
        params["chat_id"] = chat_id
    if method in {"getUserProfilePhotos", "getUserProfileAudios", "getUserChatBoosts",
                  "verifyUser", "removeUserVerification"}:
        params["user_id"] = user_id
    if method in {"getChatMember", "approveChatJoinRequest", "declineChatJoinRequest",
                  "banChatMember", "unbanChatMember", "restrictChatMember",
                  "promoteChatMember", "setChatAdministratorCustomTitle"}:
        params["user_id"] = user_id
    if method in {"getUserProfilePhotos"}:
        params.update(offset=0, limit=1)
    if method in {"setChatTitle"}:
        params["title"] = "PTB probe"
    if method in {"setChatDescription"}:
        params["description"] = "PTB probe"
    if method in {"setChatMenuButton"}:
        params["menu_button"] = {"type": "default"}
    if method in {"setChatPhoto"}:
        params["photo"] = file_id
    if method in {"setChatPermissions"}:
        params["permissions"] = {"can_send_messages": True}
    if method in {"setChatAdministratorCustomTitle"}:
        params.update(user_id=user_id, custom_title="PTB probe")
    if method in {"banChatSenderChat", "unbanChatSenderChat"}:
        params["sender_chat_id"] = chat_id
    if method in {"pinChatMessage", "unpinChatMessage", "deleteMessage", "deleteMessages",
                  "deleteMessageReaction", "deleteAllMessageReactions", "setMessageReaction",
                  "editMessageText", "editMessageCaption", "editMessageMedia", "editMessageReplyMarkup",
                  "editMessageLiveLocation", "stopMessageLiveLocation", "editMessageChecklist",
                  "stopPoll", "editEphemeralMessageText", "editEphemeralMessageMedia",
                  "editEphemeralMessageCaption", "editEphemeralMessageReplyMarkup", "deleteEphemeralMessage",
                  "approveSuggestedPost", "declineSuggestedPost", "forwardMessage", "copyMessage",
                  "forwardMessages", "copyMessages", "setGameScore", "getGameHighScores"}:
        params.update(chat_id=chat_id, message_id=message_id)
    if method in {"unpinAllChatMessages", "leaveChat", "setChatPermissions", "setChatAdministratorCustomTitle",
                  "banChatMember", "banChatSenderChat", "unbanChatSenderChat", "unbanChatMember",
                  "restrictChatMember", "promoteChatMember", "setChatPhoto", "setChatStickerSet",
                  "getUserChatBoosts"}:
        params["chat_id"] = chat_id
    if method in {"deleteMessages"}:
        params["message_ids"] = [message_id]
    if method in {"deleteMessageReaction", "deleteAllMessageReactions", "setMessageReaction",
                  "setGameScore", "getGameHighScores"}:
        params["user_id"] = user_id
    if method in {"editMessageText", "editEphemeralMessageText", "editMessageChecklist"}:
        params["text"] = "PTB probe edit"
    if method in {"editMessageCaption", "editEphemeralMessageCaption"}:
        params["caption"] = "PTB probe edit"
    if method in {"editMessageMedia"}:
        params["media"] = {"type": "photo", "media": file_id}
    if method in {"editMessageLiveLocation"}:
        params.update(latitude=1.0, longitude=1.0)
    if method in {"forwardMessage", "copyMessage"}:
        params["from_chat_id"] = chat_id
    if method in {"forwardMessages", "copyMessages"}:
        params.update(from_chat_id=chat_id, message_ids=[message_id])
    if method in {"setGameScore", "getGameHighScores"}:
        params["game_short_name"] = "ptb_probe"
    if method == "setGameScore":
        params["score"] = 1
    if method in {"getCustomEmojiStickers"}:
        params["custom_emoji_ids"] = ["invalid-custom-emoji"]
    if method in {"getStickerSet", "deleteStickerSet"}:
        params["name"] = "ptb_probe_by_teamgram"
    if method in {"createNewStickerSet"}:
        params.update(user_id=user_id, name="ptb_probe_by_teamgram", title="PTB probe", stickers=[])
    if method in {"addStickerToSet", "setStickerPositionInSet", "deleteStickerFromSet",
                  "replaceStickerInSet", "setStickerEmojiList", "setStickerKeywords",
                  "setStickerMaskPosition", "setStickerSetTitle", "setStickerSetThumbnail",
                  "setCustomEmojiStickerSetThumbnail"}:
        params.update(user_id=user_id, name="ptb_probe_by_teamgram", sticker=file_id)
    if method == "getStarTransactions":
        params.update(offset="", limit=1)
    if method == "refundStarPayment":
        params.update(
            user_id=user_id,
            telegram_payment_charge_id=os.getenv("BOT_PROBE_STAR_CHARGE_ID", "ptb-probe"),
        )
    if method == "editUserStarSubscription":
        params.update(
            user_id=user_id,
            telegram_payment_charge_id=os.getenv("BOT_PROBE_STAR_SUBSCRIPTION_ID", "ptb-probe"),
            is_canceled=True,
        )
    if method == "createInvoiceLink":
        params.update(
            title="PTB probe",
            description="probe",
            payload=os.getenv("BOT_PROBE_INVOICE_PAYLOAD", "ptb-probe-xtr-link"),
            provider_token=os.getenv("BOT_PROBE_PROVIDER_TOKEN", ""),
            currency=os.getenv("BOT_PROBE_INVOICE_CURRENCY", "XTR"),
            prices=[{"label": "probe", "amount": 1}],
        )
    if method == "setMyCommands":
        # PTB 22.8 accepts `(command, description)` pairs or BotCommand
        # instances here and serializes them to the Bot API object shape.
        params["commands"] = [("start", "start")]
    if method in {"setMyName", "setMyDescription", "setMyShortDescription"}:
        params["name" if method == "setMyName" else "description"] = "PTB probe"
    if method == "setMyDefaultAdministratorRights":
        params["rights"] = {}
    if method == "setMyProfilePhoto":
        params["photo"] = file_id
    if method == "setUserEmojiStatus":
        params.update(user_id=user_id, emoji_status_custom_emoji_id="invalid-custom-emoji")
    if method in {"getBusinessConnection", "readBusinessMessage", "deleteBusinessMessages"}:
        params["business_connection_id"] = business_connection_id
    if method == "readBusinessMessage":
        params.update(user_chat_id=chat_id, message_id=message_id)
    if method == "deleteBusinessMessages":
        params["message_ids"] = [message_id]
    if method == "answerCallbackQuery":
        params.update(callback_query_id="ptb-probe", text="probe")
    if method == "answerInlineQuery":
        params.update(inline_query_id="ptb-probe", results=[])
    if method == "answerWebAppQuery":
        params.update(web_app_query_id="ptb-probe", result={"type": "article", "id": "ptb-probe",
                                                              "title": "probe",
                                                              "input_message_content": {"message_text": "probe"}})
    if method == "answerShippingQuery":
        params.update(shipping_query_id="ptb-probe", ok=False, error_message="probe")
    if method == "answerPreCheckoutQuery":
        params.update(pre_checkout_query_id="ptb-probe", ok=False, error_message="probe")
    if method in {"answerChatJoinRequestQuery", "answerGuestQuery"}:
        params["query_id"] = "ptb-probe"
    if method in {"getManagedBotToken", "replaceManagedBotToken", "getManagedBotAccessSettings",
                  "setManagedBotAccessSettings"}:
        params["managed_bot_user_id"] = user_id
    if method in {"setBusinessAccountName", "setBusinessAccountUsername", "setBusinessAccountBio"}:
        params.update(business_connection_id=business_connection_id, first_name="PTB probe", name="PTB probe",
                      username="ptb_probe", bio="PTB probe")
    if method in {"setBusinessAccountProfilePhoto", "removeBusinessAccountProfilePhoto"}:
        params.update(business_connection_id=business_connection_id, photo=file_id)
    if method in {"convertGiftToStars", "upgradeGift", "transferGift"}:
        params.update(business_connection_id=business_connection_id, owned_gift_id=owned_gift_id)
    if method in {"savePreparedInlineMessage", "savePreparedKeyboardButton"}:
        params.update(user_id=user_id, result={"type": "article", "id": "ptb-probe", "title": "probe"})
    if method == "uploadStickerFile":
        params.update(user_id=user_id, sticker=file_id, sticker_format="static")
    if method == "setPassportDataErrors":
        params.update(user_id=user_id, errors=[])
    if method == "setBusinessAccountGiftSettings":
        params.update(
            business_connection_id=business_connection_id,
            show_gift_button=True,
            accepted_gift_types={
                "unlimited_gifts": True,
                "limited_gifts": True,
                "unique_gifts": True,
                "premium_subscription": True,
                "gifts_from_channels": True,
            },
        )
    if method == "editEphemeralMessageMedia":
        params["media"] = {"type": "photo", "media": file_id}

    # Keep the probe arguments aligned with PTB 22.8's typed signatures. The
    # Bot API names for these methods differ from the shared message and
    # sticker mutation fields above; passing the extra fields makes PTB raise a
    # local TypeError before it can send the request to Teamgram.
    if method in {"forwardMessages", "copyMessages", "deleteMessages"}:
        params.pop("message_id", None)
    if method in {"setMessageReaction"}:
        params.pop("user_id", None)
        params.setdefault("reaction", [])
    if method in {"setGameScore", "getGameHighScores"}:
        params.pop("game_short_name", None)
    if method == "sendVenue":
        params.setdefault("address", "local")
    if method == "setMyShortDescription":
        params.pop("description", None)
        params["short_description"] = "PTB probe"
    if method == "setBusinessAccountName":
        params.pop("name", None)
        params.pop("username", None)
        params.pop("bio", None)
        params["last_name"] = "Bot"
    if method == "setBusinessAccountUsername":
        params.pop("first_name", None)
        params.pop("name", None)
        params.pop("bio", None)
    if method == "setBusinessAccountBio":
        params.pop("first_name", None)
        params.pop("name", None)
        params.pop("username", None)
    if method == "removeBusinessAccountProfilePhoto":
        params.pop("photo", None)
        params["is_public"] = False
    if method == "setBusinessAccountProfilePhoto":
        params["is_public"] = False
        params["photo"] = {"type": "static", "photo": "attach://photo"}
    if method == "deleteStickerFromSet":
        params.pop("user_id", None)
        params.pop("name", None)
    if method in {"setStickerKeywords", "setStickerMaskPosition"}:
        params.pop("user_id", None)
        params.pop("name", None)
    if method == "setStickerPositionInSet":
        params.pop("user_id", None)
        params.pop("name", None)
        params["position"] = 0
    if method == "replaceStickerInSet":
        params["old_sticker"] = file_id
    if method == "setStickerEmojiList":
        params.pop("user_id", None)
        params.pop("name", None)
        params["emoji_list"] = ["🙂"]
    if method == "setStickerSetTitle":
        params.pop("user_id", None)
        params.pop("sticker", None)
        params["title"] = "PTB probe"
    if method == "setStickerSetThumbnail":
        params.pop("sticker", None)
        params["format"] = "static"
        params["thumbnail"] = file_id
    if method == "setCustomEmojiStickerSetThumbnail":
        params.pop("user_id", None)
        params.pop("sticker", None)
        params["custom_emoji_id"] = "invalid-custom-emoji"
    if method in {"setStickerKeywords"}:
        params["keywords"] = []
    if method == "setStickerMaskPosition":
        params["mask_position"] = {"point": "forehead", "x_shift": 0, "y_shift": 0, "scale": 1}
    if method in {"editChatInviteLink", "revokeChatInviteLink"}:
        params["invite_link"] = "https://t.me/+ptb-probe"
    if method in {"createForumTopic"}:
        params["name"] = "PTB probe"
    if method in {"editForumTopic", "closeForumTopic", "reopenForumTopic", "deleteForumTopic",
                  "unpinAllForumTopicMessages"}:
        params["message_thread_id"] = message_id or 1
    if method == "editGeneralForumTopic":
        params["name"] = "PTB probe"
    if method == "restrictChatMember":
        params["permissions"] = {"can_send_messages": True}
    if method == "readBusinessMessage":
        params.pop("user_chat_id", None)
        params["chat_id"] = chat_id
    if method == "setChatStickerSet":
        params["sticker_set_name"] = "ptb_probe_by_teamgram"
    if method == "setChatMemberTag":
        params.update(chat_id=chat_id, user_id=user_id, tag="PTB")
    if method == "createChatSubscriptionInviteLink":
        params.update(chat_id=chat_id, subscription_period=2592000, subscription_price=1)
    if method == "editChatSubscriptionInviteLink":
        params.update(chat_id=chat_id, invite_link="https://t.me/+ptb-probe")
    if method == "answerChatJoinRequestQuery":
        params.update(chat_join_request_query_id="1", result="decline")
    if method == "answerGuestQuery":
        params.update(guest_query_id="1", result={"type": "article", "id": "ptb-probe",
                                                   "title": "probe",
                                                   "input_message_content": {"message_text": "probe"}})
    if method == "sendChatJoinRequestWebApp":
        params.update(chat_join_request_query_id="1", web_app_url="https://example.com/ptb-probe")
    if method in {"getManagedBotToken", "replaceManagedBotToken", "getManagedBotAccessSettings",
                  "setManagedBotAccessSettings"}:
        params["user_id"] = user_id
    if method == "setManagedBotAccessSettings":
        params.update(is_access_restricted=False, user_ids=[])
    if method == "sendLivePhoto":
        params.update(chat_id=chat_id, photo=os.getenv("BOT_PROBE_PHOTO_ID", file_id),
                      live_photo=os.getenv("BOT_PROBE_VIDEO_FILE_ID", file_id))
    if method == "sendMessageDraft":
        params.update(chat_id=chat_id, draft_id=message_id or 1, text="PTB draft probe")
    if method == "sendRichMessage":
        params.pop("text", None)
        params.update(chat_id=chat_id, rich_message={"html": "<b>PTB rich probe</b>"})
    if method == "sendRichMessageDraft":
        params.pop("text", None)
        params.update(chat_id=chat_id, draft_id=message_id or 1,
                      rich_message={"html": "<b>PTB rich draft probe</b>"})
    if method == "setBusinessAccountGiftSettings":
        params.update(business_connection_id=business_connection_id, show_gift_button=True,
                      accepted_gift_types={"unlimited_gifts": True, "limited_gifts": True,
                                           "unique_gifts": True, "premium_subscription": True,
                                           "gifts_from_channels": True})
    if method in {"getBusinessAccountStarBalance", "getBusinessAccountGifts",
                  "transferBusinessAccountStars"}:
        params["business_connection_id"] = business_connection_id
    if method == "getBusinessAccountGifts":
        params.update(exclude_unsaved=False, exclude_saved=False, exclude_unlimited=False,
                      exclude_unique=False, sort_by_price=False, offset="", limit=1,
                      exclude_limited_upgradable=False,
                      exclude_limited_non_upgradable=False,
                      exclude_from_blockchain=False)
    if method == "transferBusinessAccountStars":
        params["star_count"] = 1
    if method == "getUserGifts":
        params.pop("chat_id", None)
        params.update(user_id=user_id, offset="", limit=1)
    if method == "sendGift":
        params.update(gift_id=gift_id, user_id=user_id, pay_for_upgrade=False)
    if method == "giftPremiumSubscription":
        params.update(user_id=user_id, month_count=3, star_count=1000)
    if method in {"convertGiftToStars", "upgradeGift", "transferGift"}:
        params.update(business_connection_id=business_connection_id, owned_gift_id=owned_gift_id)
    if method == "upgradeGift":
        params.update(keep_original_details=False, star_count=gift_star_count)
    if method == "transferGift":
        params.update(new_owner_chat_id=chat_id, star_count=gift_star_count)
    if method in {"editEphemeralMessageText", "editEphemeralMessageMedia",
                  "editEphemeralMessageCaption", "editEphemeralMessageReplyMarkup",
                  "deleteEphemeralMessage"}:
        params.pop("message_id", None)
        params.update(receiver_user_id=user_id, ephemeral_message_id=message_id or 1)
    if method == "savePreparedKeyboardButton":
        params.pop("result", None)
        params.update(user_id=user_id, button={"text": "PTB probe",
                                               "request_users": {"request_id": 1}})
    if method == "editMessageChecklist":
        params.pop("text", None)
        params.update(business_connection_id=business_connection_id, checklist={
            "title": "PTB probe", "tasks": [{"id": 1, "text": "probe"}]})
    if method in {"postStory", "editStory", "deleteStory"}:
        params["business_connection_id"] = business_connection_id
    if method == "postStory":
        params.update(active_period=86400, content={"type": "photo", "photo": "attach://photo"})
    if method == "repostStory":
        params.update(active_period=86400, from_chat_id=chat_id, from_story_id=1,
                      post_to_chat_page=False)
    if method == "editStory":
        params.update(story_id=1, content={"type": "photo", "photo": "attach://photo"})
    if method == "deleteStory":
        params["story_id"] = 1
    return params


async def call_method(bot: Any, method: str) -> MethodProbeResult:
    """Call one endpoint through PTB's typed method or generic escape hatch."""

    params = _probe_params(method)
    typed = getattr(bot, snake_case(method), None)
    # PTB's typed wrappers are the primary path. A wrapper is usable when the
    # probe has supplied every required argument; otherwise the generic PTB
    # escape hatch still sends the official endpoint request.
    typed_ready = False
    if callable(typed):
        signature = inspect.signature(typed)
        required = {
            parameter.name
            for parameter in signature.parameters.values()
            if parameter.name != "self"
            and parameter.default is inspect.Parameter.empty
            and parameter.kind in (inspect.Parameter.POSITIONAL_ONLY,
                                   inspect.Parameter.POSITIONAL_OR_KEYWORD,
                                   inspect.Parameter.KEYWORD_ONLY)
        }
        typed_ready = required.issubset(params)
    try:
        if typed_ready:
            try:
                await typed(**params)
                return MethodProbeResult(method, f"ptb.{snake_case(method)}", "ok")
            except Exception as exc:  # noqa: BLE001 - classify local PTB validation below
                # TelegramError means the endpoint was reached and returned a
                # canonical Bot API error; retrying a mutation could duplicate
                # it. Local argument validation errors, however, must still
                # reach Teamgram so every catalogued method is exercised.
                from telegram.error import TelegramError

                if isinstance(exc, TelegramError) or not (
                    isinstance(exc, TypeError) and "unexpected keyword argument" in str(exc)
                ):
                    raise
                await bot.do_api_request(method, api_kwargs=params)
                return MethodProbeResult(method, "ptb.do_api_request (typed fallback)", "ok", category="success")
        await bot.do_api_request(method, api_kwargs=params)
        return MethodProbeResult(method, "ptb.do_api_request", "ok", category="success")
    except Exception as exc:  # noqa: BLE001 - the report is the interface
        from telegram.error import BadRequest, NetworkError, TelegramError

        detail = str(exc)
        if method in TERMINAL_METHODS:
            category = "terminal"
        elif isinstance(exc, BadRequest):
            provider_markers = (
                "provider",
                "payment provider",
                "business provider",
                "not configured",
                "unavailable",
            )
            category = (
                "provider_dependency"
                if any(marker in detail.lower() for marker in provider_markers)
                else "telegram_business_error"
            )
        elif isinstance(exc, (NetworkError, TelegramError)):
            category = "local_error"
        else:
            category = "local_error"
        return MethodProbeResult(method, f"ptb.{snake_case(method)}" if typed_ready else "ptb.do_api_request",
                                 "error", detail, category)


def method_page_text(page: int = 0) -> str:
    page_count = (len(OFFICIAL_METHODS) + METHOD_PAGE_SIZE - 1) // METHOD_PAGE_SIZE
    page = max(0, min(page, page_count - 1))
    start = page * METHOD_PAGE_SIZE + 1
    end = min(len(OFFICIAL_METHODS), start + METHOD_PAGE_SIZE - 1)
    return f"PTB Bot API surface: 185 official methods (showing {start}-{end}/{len(OFFICIAL_METHODS)})."


def method_keyboard(page: int = 0) -> InlineKeyboardMarkup:
    page_count = (len(OFFICIAL_METHODS) + METHOD_PAGE_SIZE - 1) // METHOD_PAGE_SIZE
    page = max(0, min(page, page_count - 1))
    methods = OFFICIAL_METHODS[page * METHOD_PAGE_SIZE:(page + 1) * METHOD_PAGE_SIZE]
    rows = [[InlineKeyboardButton(name, callback_data=f"call:{name}")] for name in methods]
    navigation: list[InlineKeyboardButton] = []
    if page > 0:
        navigation.append(InlineKeyboardButton("Previous", callback_data=f"surface:{page - 1}"))
    if page + 1 < page_count:
        navigation.append(InlineKeyboardButton("Next", callback_data=f"surface:{page + 1}"))
    if navigation:
        rows.append(navigation)
    return InlineKeyboardMarkup(rows)


def menu() -> InlineKeyboardMarkup:
    return InlineKeyboardMarkup(
        [[InlineKeyboardButton("Ping bot", callback_data="ping")]]
    )


def reply_menu() -> ReplyKeyboardMarkup:
    keyboard: list[list[Any]] = [
        ["Keyboard ping", "Help"],
        ["Show 185 methods", "Run 185 API smoke"],
    ]
    mini_app_url = os.getenv("BOT_MINI_APP_URL", "").strip()
    if mini_app_url:
        keyboard.append([KeyboardButton("Open Mini App", web_app=WebAppInfo(url=mini_app_url))])
    return ReplyKeyboardMarkup(
        keyboard,
        resize_keyboard=True,
        is_persistent=True,
        input_field_placeholder="Choose an action",
    )


async def start(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    del context
    if update.effective_message is None:
        return
    await update.effective_message.reply_text(
        "PTB bot is connected to Teamgram. Send a message or click the button.",
        reply_markup=reply_menu(),
    )
    await update.effective_message.reply_text("Inline controls:", reply_markup=menu())
    LOGGER.info("handled /start update_id=%s", update.update_id)


async def _edit_or_reply(
    query: Any,
    bot: Any,
    text: str,
    reply_markup: Any = None,
) -> None:
    """Keep callback handlers usable when a client omits message contents."""

    try:
        await query.edit_message_text(text, reply_markup=reply_markup)
    except TypeError as exc:
        if "inaccessible message" not in str(exc) or query.from_user is None:
            raise
        await bot.send_message(
            chat_id=query.from_user.id,
            text=text,
            reply_markup=reply_markup,
        )
    except BadRequest as exc:
        detail = str(exc).lower()
        edit_failures = (
            "message_author_required",
            "message to edit not found",
            "message can't be edited",
            "inaccessible message",
        )
        if query.from_user is None or not any(error in detail for error in edit_failures):
            raise
        await bot.send_message(
            chat_id=query.from_user.id,
            text=text,
            reply_markup=reply_markup,
        )


async def button(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    query = update.callback_query
    if query is None:
        return
    await query.answer("Callback reached the PTB bot")
    await _edit_or_reply(query, context.bot, "Callback reached the PTB bot.")
    LOGGER.info("handled callback update_id=%s data=%s", update.update_id, query.data)


async def methods(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    del context
    if update.effective_message is None:
        return
    await update.effective_message.reply_text(method_page_text(), reply_markup=method_keyboard())


async def smoke(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    if update.effective_message is None:
        return
    include_lifecycle = os.getenv("BOT_SMOKE_INCLUDE_LIFECYCLE", "").lower() in {"1", "true", "yes"}
    await update.effective_message.reply_text(
        f"Running {len(OFFICIAL_METHODS)} PTB requests ({len(OFFICIAL_METHODS)} official methods catalogued). This can take a moment..."
    )
    results: list[MethodProbeResult] = []
    for method in NON_TERMINAL_METHODS:
        result = await call_method(context.bot, method)
        results.append(result)
        LOGGER.info("method=%s route=%s status=%s", result.method, result.route, result.status)
    counts = Counter(result.status for result in results)
    categories = Counter(result.category for result in results)
    errors = [result.method for result in results if result.status == "error"]
    summary = f"PTB smoke complete. requested={len(results)} ok={counts['ok']} error={counts['error']}"
    deferred_terminal_count = 0 if include_lifecycle else len(TERMINAL_METHODS)
    summary += (
        f"\nCategories: success={categories['success']} "
        f"telegram_business_error={categories['telegram_business_error']} "
        f"provider_dependency={categories['provider_dependency']} "
        f"local_error={categories['local_error']} "
        f"terminal={deferred_terminal_count}"
    )
    if include_lifecycle:
        summary += (
            "\n183 non-terminal methods completed. The two terminal methods are now called last; "
            "Telegram may invalidate this bot token after logOut or close."
        )
    else:
        summary += (
            f"\nOfficial catalog={len(OFFICIAL_METHODS)}; deferred terminal methods: "
            "logOut, close. Click either method in the catalog to test it individually."
        )
    summary += "\nEvery requested method used a PTB typed wrapper when available, otherwise ptb.do_api_request."
    if errors:
        summary += "\nFirst errors: " + ", ".join(errors[:12])
    await update.effective_message.reply_text(summary, reply_markup=method_keyboard())
    if include_lifecycle:
        for method in TERMINAL_METHOD_ORDER:
            result = await call_method(context.bot, method)
            LOGGER.info("method=%s route=%s status=%s", result.method, result.route, result.status)


async def method_page(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    query = update.callback_query
    if query is None:
        return
    page = int((query.data or "surface:0").split(":", 1)[1])
    await query.answer()
    await _edit_or_reply(query, context.bot, method_page_text(page), method_keyboard(page))


async def method_probe(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    query = update.callback_query
    if query is None:
        return
    method = (query.data or "call:getMe").split(":", 1)[1]
    await query.answer("Calling the PTB endpoint")
    if method in TERMINAL_METHODS:
        await _edit_or_reply(
            query,
            context.bot,
            f"{method}: calling terminal lifecycle endpoint. The bot may stop responding after this request."
        )
        result = await call_method(context.bot, method)
        LOGGER.info("method=%s route=%s status=%s", result.method, result.route, result.status)
        return
    result = await call_method(context.bot, method)
    detail = f"\n{result.detail[:180]}" if result.detail else ""
    page = OFFICIAL_METHODS.index(method) // METHOD_PAGE_SIZE
    await _edit_or_reply(
        query,
        context.bot,
        f"{result.method}: {result.status} via {result.route}{detail}",
        method_keyboard(page),
    )


async def echo(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    if update.effective_message is None or update.effective_message.text is None:
        return
    text = update.effective_message.text
    # Teamgram's WebA client may represent a slash command as an unknown
    # entity. Keep PTB's CommandHandler path above, but recognize the command
    # text here as a compatibility fallback for that client representation.
    command = text[1:].split("@", 1)[0].split(maxsplit=1)[0] if text.startswith("/") else ""
    if command == "start":
        await start(update, context)
        return
    if command == "methods":
        await methods(update, context)
        return
    if command == "smoke":
        await smoke(update, context)
        return
    if text == "Show 185 methods":
        await methods(update, context)
        return
    if text == "Run 185 API smoke":
        await smoke(update, context)
        return
    await update.effective_message.reply_text(
        f"PTB echo: {text}",
        reply_markup=reply_menu(),
    )
    await update.effective_message.reply_text("Inline controls:", reply_markup=menu())
    LOGGER.info("handled message update_id=%s", update.update_id)


async def web_app_data(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    if update.effective_message is None or update.effective_message.web_app_data is None:
        return
    data = update.effective_message.web_app_data
    await update.effective_message.reply_text(
        f"Mini App data received: {data.data}",
        reply_markup=reply_menu(),
    )
    LOGGER.info("handled web_app_data update_id=%s button_text=%s", update.update_id, data.button_text)


def build_application() -> Application:
    token = os.environ["BOT_TOKEN"]
    builder = Application.builder().token(token)
    base_url = os.getenv("BOT_API_BASE_URL")
    base_file_url = os.getenv("BOT_API_FILE_URL")
    if base_url:
        # PTB appends "/<method>" to the parsed base URL. Accept a trailing
        # slash in deployment env files so both Telegram-shaped conventions
        # (".../bot" and ".../bot{token}/") remain usable.
        builder = builder.base_url(base_url.rstrip("/"))
    if base_file_url:
        builder = builder.base_file_url(base_file_url.rstrip("/"))
    application = builder.build()
    application.add_handler(CommandHandler("start", start))
    application.add_handler(CommandHandler("methods", methods))
    application.add_handler(CommandHandler("smoke", smoke))
    application.add_handler(CallbackQueryHandler(button, pattern="^ping$"))
    application.add_handler(CallbackQueryHandler(method_page, pattern="^surface:"))
    application.add_handler(CallbackQueryHandler(method_probe, pattern="^call:"))
    application.add_handler(MessageHandler(filters.StatusUpdate.WEB_APP_DATA, web_app_data))
    application.add_handler(MessageHandler(filters.TEXT & ~filters.COMMAND, echo))
    return application


def main() -> None:
    logging.getLogger("httpx").setLevel(logging.WARNING)
    logging.getLogger("httpcore").setLevel(logging.WARNING)
    logging.basicConfig(
        level=os.getenv("BOT_LOG_LEVEL", "INFO"),
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
    )
    build_application().run_polling(allowed_updates=Update.ALL_TYPES)


if __name__ == "__main__":
    main()
